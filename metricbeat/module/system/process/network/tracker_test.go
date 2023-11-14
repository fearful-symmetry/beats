package network

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/packetbeat/protos/applayer"
	"github.com/elastic/elastic-agent-libs/logp"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	name     string
	inputs   []CounterUpdateEvent
	expected map[int]PacketData
}

func TestPacketGetUpdate(t *testing.T) {
	testTrack := &Tracker{
		procData:   make(map[int]PacketData),
		updateChan: make(chan CounterUpdateEvent, 10),
		reqChan:    make(chan RequestCounters),
		stopChan:   make(chan struct{}),
		testmode:   true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	testTrack.Track(ctx)

	testTrack.Update(40, applayer.TransportTCP, &common.ProcessTuple{Src: common.Process{PID: 11}})

	testTrack.Update(44, applayer.TransportUDP, &common.ProcessTuple{Src: common.Process{PID: 13}})

	require.Eventually(t, func() bool { return testTrack.Get(13).Outgoing.UDP > 0 }, time.Second*10, time.Millisecond)

}

func TestGarbageCollect(t *testing.T) {
	_ = logp.DevelopmentSetup()
	testTrack := &Tracker{
		procData:   make(map[int]PacketData),
		updateChan: make(chan CounterUpdateEvent, 10),
		reqChan:    make(chan RequestCounters),
		stopChan:   make(chan struct{}, 1),
		testmode:   true,
		gctime:     time.Millisecond,
		dataMut:    sync.RWMutex{},
		loopWaiter: make(chan struct{}),
		log:        logp.L(),
	}

	gcPidFetch = func(ctx context.Context, pid int32) (bool, error) {
		if pid == 10 || pid == 1245 {
			return true, nil
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	//start with all pids filled out
	testTrack.procData = map[int]PacketData{10: {}, 1245: {}}

	// start garbage collector
	go func() {
		testTrack.garbageCollect(ctx)
	}()

	<-testTrack.loopWaiter
	testTrack.dataMut.Lock()
	require.Equal(t, map[int]PacketData{10: {}, 1245: {}}, testTrack.procData)
	// remove a pid, test again
	gcPidFetch = func(ctx context.Context, pid int32) (bool, error) {
		if pid == 10 {
			return true, nil
		}
		return false, nil
	}
	testTrack.dataMut.Unlock()
	<-testTrack.loopWaiter

	testTrack.dataMut.Lock()
	require.Equal(t, map[int]PacketData{10: {}}, testTrack.procData)
	testTrack.dataMut.Unlock()
	// gently shut down
	testTrack.Stop()
	<-testTrack.loopWaiter

}

func TestPacketUpdates(t *testing.T) {
	cases := []testCase{
		{
			name: "base-case",
			inputs: []CounterUpdateEvent{
				{
					pktLen:        40,
					TransProtocol: applayer.TransportTCP,
					Proc:          &common.ProcessTuple{Src: common.Process{PID: 11}},
				},
			},
			expected: map[int]PacketData{
				11: {Outgoing: PortsForProtocol{TCP: 40}},
			},
		},
		{
			name: "multiple-proto",
			inputs: []CounterUpdateEvent{
				{
					pktLen:        40,
					TransProtocol: applayer.TransportTCP,
					Proc:          &common.ProcessTuple{Src: common.Process{PID: 11}},
				},
				{
					pktLen:        44,
					TransProtocol: applayer.TransportUDP,
					Proc:          &common.ProcessTuple{Src: common.Process{PID: 13}},
				},
				{
					pktLen:        10,
					TransProtocol: applayer.TransportTCP,
					Proc:          &common.ProcessTuple{Src: common.Process{PID: 23}},
				},
				{
					pktLen:        70,
					TransProtocol: applayer.TransportTCP,
					Proc:          &common.ProcessTuple{Dst: common.Process{PID: 11}},
				},
				{
					pktLen:        41,
					TransProtocol: applayer.TransportTCP,
					Proc:          &common.ProcessTuple{Src: common.Process{PID: 11}},
				},
			},
			expected: map[int]PacketData{
				11: {Outgoing: PortsForProtocol{TCP: 81}, Incoming: PortsForProtocol{TCP: 70}},
				13: {Outgoing: PortsForProtocol{UDP: 44}},
				23: {Outgoing: PortsForProtocol{TCP: 10}},
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			testTrack := &Tracker{
				procData:   make(map[int]PacketData),
				updateChan: make(chan CounterUpdateEvent),
				reqChan:    make(chan RequestCounters, 10),
				stopChan:   make(chan struct{}),
				testmode:   true,
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			testTrack.Track(ctx)

			for _, input := range testCase.inputs {

				testTrack.Update(input.pktLen, input.TransProtocol, input.Proc)

			}

			testTrack.Stop()
			require.Equal(t, testCase.expected, testTrack.procData)
		})
	}
}
