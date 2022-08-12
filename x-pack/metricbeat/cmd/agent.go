package cmd

import (
	"fmt"
	"strings"

	"github.com/elastic/beats/v7/libbeat/common/reload"
	"github.com/elastic/beats/v7/x-pack/libbeat/management"
	"github.com/elastic/elastic-agent-client/v7/pkg/client"
	"github.com/elastic/elastic-agent-client/v7/pkg/proto"
)

func metricbeatCfg(rawIn *proto.UnitExpectedConfig, agentInfo *client.AgentInfo) ([]*reload.ConfigWithMeta, error) {
	modules, err := management.CreateInputsFromStreams(rawIn, "metrics", agentInfo)
	if err != nil {
		return nil, fmt.Errorf("error creating input list from raw expected config: %s", err)
	}

	// Extract the module name from the type, usually in the form system/metric
	module := strings.Split(rawIn.Type, "/")[0]

	for iter := range modules {
		modules[iter]["module"] = module
	}

	// format for the reloadable list needed bythe cm.Reload() method
	configList, err := management.CreateReloadConfigFromInputs(modules)
	if err != nil {
		return nil, fmt.Errorf("error creating reloader config: %w", err)
	}

	return configList, nil
}
