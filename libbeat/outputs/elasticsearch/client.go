// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package elasticsearch

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"

	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/libbeat/common/transport"
	"github.com/elastic/beats/v7/libbeat/common/transport/tlscommon"
	"github.com/elastic/beats/v7/libbeat/logp"
	"github.com/elastic/beats/v7/libbeat/outputs"
	"github.com/elastic/beats/v7/libbeat/outputs/outil"
	"github.com/elastic/beats/v7/libbeat/publisher"
	"github.com/elastic/beats/v7/libbeat/testing"
)

// Client is an elasticsearch client.
type Client struct {
	Connection
	tlsConfig *tlscommon.TLSConfig

	index    outputs.IndexSelector
	pipeline *outil.Selector
	params   map[string]string
	timeout  time.Duration

	// buffered bulk requests
	bulkRequ *bulkRequest

	// buffered json response reader
	json JSONReader

	// additional configs
	compressionLevel int
	proxyURL         *url.URL

	observer outputs.Observer
}

// ClientSettings contains the settings for a client.
type ClientSettings struct {
	URL                string
	Proxy              *url.URL
	ProxyDisable       bool
	TLS                *tlscommon.TLSConfig
	Username, Password string
	APIKey             string
	EscapeHTML         bool
	Parameters         map[string]string
	Headers            map[string]string
	Index              outputs.IndexSelector
	Pipeline           *outil.Selector
	Timeout            time.Duration
	CompressionLevel   int
	Observer           outputs.Observer
}

// ConnectCallback defines the type for the function to be called when the Elasticsearch client successfully connects to the cluster
type ConnectCallback func(client *Client) error

// Connection manages the connection for a given client.
type Connection struct {
	log *logp.Logger

	URL      string
	Username string
	Password string
	APIKey   string
	Headers  map[string]string

	http              *http.Client
	onConnectCallback func() error

	encoder bodyEncoder
	version common.Version
}

type bulkIndexAction struct {
	Index bulkEventMeta `json:"index" struct:"index"`
}

type bulkCreateAction struct {
	Create bulkEventMeta `json:"create" struct:"create"`
}

type bulkEventMeta struct {
	Index    string `json:"_index" struct:"_index"`
	DocType  string `json:"_type,omitempty" struct:"_type,omitempty"`
	Pipeline string `json:"pipeline,omitempty" struct:"pipeline,omitempty"`
	ID       string `json:"_id,omitempty" struct:"_id,omitempty"`
}

type bulkResultStats struct {
	acked        int // number of events ACKed by Elasticsearch
	duplicates   int // number of events failed with `create` due to ID already being indexed
	fails        int // number of failed events (can be retried)
	nonIndexable int // number of failed events (not indexable -> must be dropped)
	tooMany      int // number of events receiving HTTP 429 Too Many Requests
}

var (
	nameItems  = []byte("items")
	nameStatus = []byte("status")
	nameError  = []byte("error")
)

var (
	errExpectedItemsArray    = errors.New("expected items array")
	errExpectedItemObject    = errors.New("expected item response object")
	errExpectedStatusCode    = errors.New("expected item status code")
	errUnexpectedEmptyObject = errors.New("empty object")
	errExpectedObjectEnd     = errors.New("expected end of object")
	errTempBulkFailure       = errors.New("temporary bulk send failure")
)

const (
	defaultEventType = "doc"
)

// NewClient instantiates a new client.
func NewClient(
	s ClientSettings,
	onConnect *callbacksRegistry,
) (*Client, error) {
	var proxy func(*http.Request) (*url.URL, error)
	if !s.ProxyDisable {
		proxy = http.ProxyFromEnvironment
		if s.Proxy != nil {
			proxy = http.ProxyURL(s.Proxy)
		}
	}

	pipeline := s.Pipeline
	if pipeline != nil && pipeline.IsEmpty() {
		pipeline = nil
	}

	u, err := url.Parse(s.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse elasticsearch URL: %v", err)
	}
	if u.User != nil {
		s.Username = u.User.Username()
		s.Password, _ = u.User.Password()
		u.User = nil

		// Re-write URL without credentials.
		s.URL = u.String()
	}

	log := logp.NewLogger(logSelector)
	log.Infof("Elasticsearch url: %s", s.URL)

	// TODO: add socks5 proxy support
	var dialer, tlsDialer transport.Dialer

	dialer = transport.NetDialer(s.Timeout)
	tlsDialer, err = transport.TLSDialer(dialer, s.TLS, s.Timeout)
	if err != nil {
		return nil, err
	}

	if st := s.Observer; st != nil {
		dialer = transport.StatsDialer(dialer, st)
		tlsDialer = transport.StatsDialer(tlsDialer, st)
	}

	params := s.Parameters
	bulkRequ, err := newBulkRequest(s.URL, "", "", params, nil)
	if err != nil {
		return nil, err
	}

	var encoder bodyEncoder
	compression := s.CompressionLevel
	if compression == 0 {
		encoder = newJSONEncoder(nil, s.EscapeHTML)
	} else {
		encoder, err = newGzipEncoder(compression, nil, s.EscapeHTML)
		if err != nil {
			return nil, err
		}
	}

	client := &Client{
		Connection: Connection{
			log:      log,
			URL:      s.URL,
			Username: s.Username,
			Password: s.Password,
			APIKey:   base64.StdEncoding.EncodeToString([]byte(s.APIKey)),
			Headers:  s.Headers,
			http: &http.Client{
				Transport: &http.Transport{
					Dial:            dialer.Dial,
					DialTLS:         tlsDialer.Dial,
					TLSClientConfig: s.TLS.ToConfig(),
					Proxy:           proxy,
				},
				Timeout: s.Timeout,
			},
			encoder: encoder,
		},
		tlsConfig: s.TLS,
		index:     s.Index,
		pipeline:  pipeline,
		params:    params,
		timeout:   s.Timeout,

		bulkRequ: bulkRequ,

		compressionLevel: compression,
		proxyURL:         s.Proxy,
		observer:         s.Observer,
	}

	client.Connection.onConnectCallback = func() error {
		globalCallbackRegistry.mutex.Lock()
		defer globalCallbackRegistry.mutex.Unlock()

		for _, callback := range globalCallbackRegistry.callbacks {
			err := callback(client)
			if err != nil {
				return err
			}
		}

		if onConnect != nil {
			onConnect.mutex.Lock()
			defer onConnect.mutex.Unlock()

			for _, callback := range onConnect.callbacks {
				err := callback(client)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	return client, nil
}

// Clone clones a client.
func (client *Client) Clone() *Client {
	// when cloning the connection callback and params are not copied. A
	// client's close is for example generated for topology-map support. With params
	// most likely containing the ingest node pipeline and default callback trying to
	// create install a template, we don't want these to be included in the clone.

	c, _ := NewClient(
		ClientSettings{
			URL:      client.URL,
			Index:    client.index,
			Pipeline: client.pipeline,
			Proxy:    client.proxyURL,
			// Without the following nil check on proxyURL, a nil Proxy field will try
			// reloading proxy settings from the environment instead of leaving them
			// empty.
			ProxyDisable:     client.proxyURL == nil,
			TLS:              client.tlsConfig,
			Username:         client.Username,
			Password:         client.Password,
			APIKey:           client.APIKey,
			Parameters:       nil, // XXX: do not pass params?
			Headers:          client.Headers,
			Timeout:          client.http.Timeout,
			CompressionLevel: client.compressionLevel,
		},
		nil, // XXX: do not pass connection callback?
	)
	return c
}

func (client *Client) Publish(batch publisher.Batch) error {
	events := batch.Events()
	rest, err := client.publishEvents(events)
	if len(rest) == 0 {
		batch.ACK()
	} else {
		batch.RetryEvents(rest)
	}
	return err
}

// PublishEvents sends all events to elasticsearch. On error a slice with all
// events not published or confirmed to be processed by elasticsearch will be
// returned. The input slice backing memory will be reused by return the value.
func (client *Client) publishEvents(
	data []publisher.Event,
) ([]publisher.Event, error) {
	begin := time.Now()
	st := client.observer

	if st != nil {
		st.NewBatch(len(data))
	}

	if len(data) == 0 {
		return nil, nil
	}

	body := client.encoder
	body.Reset()

	// encode events into bulk request buffer, dropping failed elements from
	// events slice

	eventType := ""
	if client.GetVersion().Major < 7 {
		eventType = defaultEventType
	}

	origCount := len(data)
	data = bulkEncodePublishRequest(client.Connection.log, client.GetVersion(), body, client.index, client.pipeline, eventType, data)
	newCount := len(data)
	if st != nil && origCount > newCount {
		st.Dropped(origCount - newCount)
	}
	if newCount == 0 {
		return nil, nil
	}

	requ := client.bulkRequ
	requ.Reset(body)
	status, result, sendErr := client.sendBulkRequest(requ)
	if sendErr != nil {
		client.Connection.log.Error("Failed to perform any bulk index operations: %+v", sendErr)
		return data, sendErr
	}

	client.Connection.log.Debugf("PublishEvents: %d events have been published to elasticsearch in %v.",
		len(data),
		time.Now().Sub(begin))

	// check response for transient errors
	var failedEvents []publisher.Event
	var stats bulkResultStats
	if status != 200 {
		failedEvents = data
		stats.fails = len(failedEvents)
	} else {
		client.json.init(result)
		failedEvents, stats = bulkCollectPublishFails(client.Connection.log, &client.json, data)
	}

	failed := len(failedEvents)
	if st := client.observer; st != nil {
		dropped := stats.nonIndexable
		duplicates := stats.duplicates
		acked := len(data) - failed - dropped - duplicates

		st.Acked(acked)
		st.Failed(failed)
		st.Dropped(dropped)
		st.Duplicate(duplicates)
		st.ErrTooMany(stats.tooMany)
	}

	if failed > 0 {
		if sendErr == nil {
			sendErr = errTempBulkFailure
		}
		return failedEvents, sendErr
	}
	return nil, nil
}

// fillBulkRequest encodes all bulk requests and returns slice of events
// successfully added to bulk request.
func bulkEncodePublishRequest(
	log *logp.Logger,
	version common.Version,
	body bulkWriter,
	index outputs.IndexSelector,
	pipeline *outil.Selector,
	eventType string,
	data []publisher.Event,
) []publisher.Event {
	okEvents := data[:0]
	for i := range data {
		event := &data[i].Content
		meta, err := createEventBulkMeta(log, version, index, pipeline, eventType, event)
		if err != nil {
			log.Errorf("Failed to encode event meta data: %+v", err)
			continue
		}
		if err := body.Add(meta, event); err != nil {
			log.Errorf("Failed to encode event: %+v", err)
			log.Debugf("Failed event: %v", event)
			continue
		}
		okEvents = append(okEvents, data[i])
	}
	return okEvents
}

func createEventBulkMeta(
	log *logp.Logger,
	version common.Version,
	indexSel outputs.IndexSelector,
	pipelineSel *outil.Selector,
	eventType string,
	event *beat.Event,
) (interface{}, error) {
	pipeline, err := getPipeline(event, pipelineSel)
	if err != nil {
		err := fmt.Errorf("failed to select pipeline: %v", err)
		return nil, err
	}

	index, err := indexSel.Select(event)
	if err != nil {
		err := fmt.Errorf("failed to select event index: %v", err)
		return nil, err
	}

	var id string
	if m := event.Meta; m != nil {
		if tmp := m["_id"]; tmp != nil {
			if s, ok := tmp.(string); ok {
				id = s
			} else {
				log.Errorf("Event ID '%v' is no string value", id)
			}
		}
	}

	meta := bulkEventMeta{
		Index:    index,
		DocType:  eventType,
		Pipeline: pipeline,
		ID:       id,
	}

	if id != "" || version.Major > 7 || (version.Major == 7 && version.Minor >= 5) {
		return bulkCreateAction{meta}, nil
	}
	return bulkIndexAction{meta}, nil
}

func getPipeline(event *beat.Event, pipelineSel *outil.Selector) (string, error) {
	if event.Meta != nil {
		if pipeline, exists := event.Meta["pipeline"]; exists {
			if p, ok := pipeline.(string); ok {
				return p, nil
			}
			return "", errors.New("pipeline metadata is no string")
		}
	}

	if pipelineSel != nil {
		return pipelineSel.Select(event)
	}
	return "", nil
}

// bulkCollectPublishFails checks per item errors returning all events
// to be tried again due to error code returned for that items. If indexing an
// event failed due to some error in the event itself (e.g. does not respect mapping),
// the event will be dropped.
func bulkCollectPublishFails(
	log *logp.Logger,
	reader *JSONReader,
	data []publisher.Event,
) ([]publisher.Event, bulkResultStats) {
	if err := BulkReadToItems(reader); err != nil {
		log.Errorf("failed to parse bulk response: %+v", err)
		return nil, bulkResultStats{}
	}

	count := len(data)
	failed := data[:0]
	stats := bulkResultStats{}
	for i := 0; i < count; i++ {
		status, msg, err := BulkReadItemStatus(log, reader)
		if err != nil {
			log.Error(err)
			return nil, bulkResultStats{}
		}

		if status < 300 {
			stats.acked++
			continue // ok value
		}

		if status == 409 {
			// 409 is used to indicate an event with same ID already exists if
			// `create` op_type is used.
			stats.duplicates++
			continue // ok
		}

		if status < 500 {
			if status == http.StatusTooManyRequests {
				stats.tooMany++
			} else {
				// hard failure, don't collect
				log.Warnf("Cannot index event %#v (status=%v): %s", data[i], status, msg)
				stats.nonIndexable++
				continue
			}
		}

		log.Debugf("Bulk item insert failed (i=%v, status=%v): %s", i, status, msg)
		stats.fails++
		failed = append(failed, data[i])
	}

	return failed, stats
}

// BulkReadToItems reads the bulk response up to (but not including) items
func BulkReadToItems(reader *JSONReader) error {
	if err := reader.ExpectDict(); err != nil {
		return errExpectedObject
	}

	// find 'items' field in response
	for {
		kind, name, err := reader.nextFieldName()
		if err != nil {
			return err
		}

		if kind == dictEnd {
			return errExpectedItemsArray
		}

		// found items array -> continue
		if bytes.Equal(name, nameItems) {
			break
		}

		reader.ignoreNext()
	}

	// check items field is an array
	if err := reader.ExpectArray(); err != nil {
		return errExpectedItemsArray
	}

	return nil
}

// BulkReadItemStatus reads the status and error fields from the bulk item
func BulkReadItemStatus(log *logp.Logger, reader *JSONReader) (int, []byte, error) {
	// skip outer dictionary
	if err := reader.ExpectDict(); err != nil {
		return 0, nil, errExpectedItemObject
	}

	// find first field in outer dictionary (e.g. 'create')
	kind, _, err := reader.nextFieldName()
	parserErr := func(err error) error {
		return errors.Wrapf(err, "Failed to parse bulk response item")
	}
	if err != nil {
		return 0, nil, parserErr(err)
	}
	if kind == dictEnd {
		err = errUnexpectedEmptyObject
		return 0, nil, parserErr(err)
	}

	// parse actual item response code and error message
	status, msg, err := itemStatusInner(log, reader)
	if err != nil {
		return 0, nil, parserErr(err)
	}

	// close dictionary. Expect outer dictionary to have only one element
	kind, _, err = reader.step()
	if err != nil {
		return 0, nil, parserErr(err)
	}
	if kind != dictEnd {
		err = errExpectedObjectEnd
		return 0, nil, parserErr(err)
	}

	return status, msg, nil
}

func itemStatusInner(log *logp.Logger, reader *JSONReader) (int, []byte, error) {
	if err := reader.ExpectDict(); err != nil {
		return 0, nil, errExpectedItemObject
	}

	status := -1
	var msg []byte
	for {
		kind, name, err := reader.nextFieldName()
		if err != nil {
			log.Errorf("Failed to parse bulk response item: %+v", err)
		}
		if kind == dictEnd {
			break
		}

		switch {
		case bytes.Equal(name, nameStatus): // name == "status"
			status, err = reader.nextInt()
			if err != nil {
				return 0, nil, err
			}

		case bytes.Equal(name, nameError): // name == "error"
			msg, err = reader.ignoreNext() // collect raw string for "error" field
			if err != nil {
				return 0, nil, err
			}

		default: // ignore unknown fields
			_, err = reader.ignoreNext()
			if err != nil {
				return 0, nil, err
			}
		}
	}

	if status < 0 {
		return 0, nil, errExpectedStatusCode
	}
	return status, msg, nil
}

// LoadJSON creates a PUT request based on a JSON document.
func (client *Client) LoadJSON(path string, json map[string]interface{}) ([]byte, error) {
	status, body, err := client.Request("PUT", path, "", nil, json)
	if err != nil {
		return body, fmt.Errorf("couldn't load json. Error: %s", err)
	}
	if status > 300 {
		return body, fmt.Errorf("couldn't load json. Status: %v", status)
	}

	return body, nil
}

// GetVersion returns the elasticsearch version the client is connected to.
func (client *Client) GetVersion() common.Version {
	return client.Connection.version
}

func (client *Client) Test(d testing.Driver) {
	d.Run("elasticsearch: "+client.URL, func(d testing.Driver) {
		u, err := url.Parse(client.URL)
		d.Fatal("parse url", err)

		address := u.Host

		d.Run("connection", func(d testing.Driver) {
			netDialer := transport.TestNetDialer(d, client.timeout)
			_, err = netDialer.Dial("tcp", address)
			d.Fatal("dial up", err)
		})

		if u.Scheme != "https" {
			d.Warn("TLS", "secure connection disabled")
		} else {
			d.Run("TLS", func(d testing.Driver) {
				netDialer := transport.NetDialer(client.timeout)
				tlsDialer, err := transport.TestTLSDialer(d, netDialer, client.tlsConfig, client.timeout)
				_, err = tlsDialer.Dial("tcp", address)
				d.Fatal("dial up", err)
			})
		}

		err = client.Connect()
		d.Fatal("talk to server", err)
		d.Info("version", client.version.String())
	})
}

func (client *Client) String() string {
	return "elasticsearch(" + client.Connection.URL + ")"
}

// Connect connects the client. It runs a GET request against the root URL of
// the configured host, updates the known Elasticsearch version and calls
// globally configured handlers.
func (client *Client) Connect() error {
	return client.Connection.Connect()
}

// Connect connects the client. It runs a GET request against the root URL of
// the configured host, updates the known Elasticsearch version and calls
// globally configured handlers.
func (conn *Connection) Connect() error {
	versionString, err := conn.Ping()
	if err != nil {
		return err
	}

	if version, err := common.NewVersion(versionString); err != nil {
		conn.log.Errorf("Invalid version from Elasticsearch: %s", versionString)
		conn.version = common.Version{}
	} else {
		conn.version = *version
	}

	err = conn.onConnectCallback()
	if err != nil {
		return fmt.Errorf("Connection marked as failed because the onConnect callback failed: %v", err)
	}
	return nil
}

// Ping sends a GET request to the Elasticsearch.
func (conn *Connection) Ping() (string, error) {
	conn.log.Debugf("ES Ping(url=%v)", conn.URL)

	status, body, err := conn.execRequest("GET", conn.URL, nil)
	if err != nil {
		conn.log.Debugf("Ping request failed with: %+v", err)
		return "", err
	}

	if status >= 300 {
		return "", fmt.Errorf("Non 2xx response code: %d", status)
	}

	var response struct {
		Version struct {
			Number string
		}
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", fmt.Errorf("Failed to parse JSON response: %v", err)
	}

	conn.log.Debugf("Ping status code: %v", status)
	conn.log.Infof("Attempting to connect to Elasticsearch version %s", response.Version.Number)
	return response.Version.Number, nil
}

// Close closes a connection.
func (conn *Connection) Close() error {
	return nil
}

// Request sends a request via the connection.
func (conn *Connection) Request(
	method, path string,
	pipeline string,
	params map[string]string,
	body interface{},
) (int, []byte, error) {

	url := addToURL(conn.URL, path, pipeline, params)
	conn.log.Debugf("%s %s %s %v", method, url, pipeline, body)

	return conn.RequestURL(method, url, body)
}

// RequestURL sends a request with the connection object to an alternative url
func (conn *Connection) RequestURL(
	method, url string,
	body interface{},
) (int, []byte, error) {

	if body == nil {
		return conn.execRequest(method, url, nil)
	}

	if err := conn.encoder.Marshal(body); err != nil {
		conn.log.Warnf("Failed to json encode body (%+v): %#v", err, body)
		return 0, nil, ErrJSONEncodeFailed
	}
	return conn.execRequest(method, url, conn.encoder.Reader())
}

func (conn *Connection) execRequest(
	method, url string,
	body io.Reader,
) (int, []byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		conn.log.Warnf("Failed to create request %+v", err)
		return 0, nil, err
	}
	if body != nil {
		conn.encoder.AddHeader(&req.Header)
	}
	return conn.execHTTPRequest(req)
}

func (conn *Connection) execHTTPRequest(req *http.Request) (int, []byte, error) {
	req.Header.Add("Accept", "application/json")

	if conn.Username != "" || conn.Password != "" {
		req.SetBasicAuth(conn.Username, conn.Password)
	}

	if conn.APIKey != "" {
		req.Header.Add("Authorization", "ApiKey "+conn.APIKey)
	}

	for name, value := range conn.Headers {
		req.Header.Add(name, value)
	}

	// The stlib will override the value in the header based on the configured `Host`
	// on the request which default to the current machine.
	//
	// We use the normalized key header to retrieve the user configured value and assign it to the host.
	if host := req.Header.Get("Host"); host != "" {
		req.Host = host
	}

	resp, err := conn.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer closing(conn.log, resp.Body)

	status := resp.StatusCode
	obj, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return status, nil, err
	}

	if status >= 300 {
		// add the response body with the error returned by Elasticsearch
		err = fmt.Errorf("%v: %s", resp.Status, obj)
	}

	return status, obj, err
}

// GetVersion returns the elasticsearch version the client is connected to.
// The version is read and updated on 'Connect'.
func (conn *Connection) GetVersion() common.Version {
	return conn.version
}

func closing(log *logp.Logger, c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Warnf("Close failed with: %+v", err)
	}
}
