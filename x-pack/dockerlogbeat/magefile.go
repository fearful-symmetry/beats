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

// +build mage

package main

import (
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"

	"github.com/pkg/errors"
)

var hubID = "ossifrage"
var pluginVersion = "0.0.1"
var name = "dockerlogbeat"
var containerName = name + "_container"
var dockerPluginName = hubID + "/" + name
var dockerPlugin = dockerPluginName + ":" + pluginVersion

// Build builds docker rootfs container root
func Build() error {
	mg.Deps(Clean)

	dockerLogBeatDir, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "error getting work dir")
	}

	err = os.Chdir("../..")
	if err != nil {
		return errors.Wrap(err, "error changing directory")
	}

	// err = sh.RunV("docker", "build", "--target", "builder", "-t", "rootfsimage-build", "-f", "x-pack/dockerlogbeat/Dockerfile", ".")
	// if err != nil {
	// 	return errors.Wrap(err, "error rooting rootfsimage-build")
	// }

	err = sh.RunV("docker", "build", "--target", "final", "-t", "rootfsimage", "-f", "x-pack/dockerlogbeat/Dockerfile", ".")
	if err != nil {
		return errors.Wrap(err, "error building final container image")
	}

	err = os.Chdir(dockerLogBeatDir)
	if err != nil {
		return errors.Wrap(err, "error returning to dockerlogbeat dir")
	}

	os.Mkdir("rootfs", 0755)

	err = sh.RunV("docker", "create", "--name", containerName, "rootfsimage", "true")
	if err != nil {
		return errors.Wrap(err, "error creating container")
	}

	err = sh.RunV("docker", "export", containerName, "-o", "temproot.tar")
	if err != nil {
		return errors.Wrap(err, "error exporting container")
	}

	return sh.RunV("tar", "-xf", "temproot.tar", "-C", "rootfs")
}

// Clean removes working objects and containers
func Clean() error {

	sh.RunV("docker", "rm", "-vf", containerName)
	sh.RunV("docker", "rmi", "rootfsimage")
	sh.RunV("docker", "rmi", "rootfsimage-build")
	sh.Rm("rootfs")
	sh.RunV("docker", "plugin", "disable", dockerPlugin)
	sh.RunV("docker", "plugin", "rm", dockerPlugin)

	return nil
}

func Install() error {
	err := sh.RunV("docker", "plugin", "create", dockerPlugin, ".")
	if err != nil {
		return errors.Wrap(err, "error creating plugin")
	}

	err = sh.RunV("docker", "plugin", "enable", dockerPlugin)
	if err != nil {
		return errors.Wrap(err, "error enabling plugin")
	}

	return nil
}

// Create builds and creates a docker plugin
func Create() {
	mg.SerialDeps(Build, Install)
}
