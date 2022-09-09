// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cmd

import (
	"github.com/elastic/beats/v7/libbeat/cmd"
	packetbeatCmd "github.com/elastic/beats/v7/packetbeat/cmd"
	"github.com/elastic/beats/v7/x-pack/libbeat/management"

	_ "github.com/elastic/beats/v7/x-pack/libbeat/include"

	// This registers the Npcap installer on Windows.
	_ "github.com/elastic/beats/v7/x-pack/packetbeat/npcap"
)

// Name of this beat.
var Name = packetbeatCmd.Name

// RootCmd to handle beats cli
var RootCmd *cmd.BeatsRootCmd

func init() {
	management.ConfigTransform.SetTransform(packetbeatCfg)
	settings := packetbeatCmd.PacketbeatSettings()
	settings.ElasticLicensed = true
	RootCmd = packetbeatCmd.Initialize(settings)
}
