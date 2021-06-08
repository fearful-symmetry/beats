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

package cpu

import (
	"bufio"
	"strings"

	"github.com/joeshaw/multierror"
	"github.com/pkg/errors"

	"github.com/elastic/beats/v7/metricbeat/internal/metrics"
)

func scanStatFile(scanner *bufio.Scanner) (CPUMetrics, error) {
	cpuData, err := statScanner(scanner, parseCPULine)
	if err != nil {
		return CPUMetrics{}, errors.Wrap(err, "error scanning stat file")
	}
	return cpuData, nil
}

func parseCPULine(line string) (CPU, error) {
	cpuData := CPU{}
	fields := strings.Fields(line)
	var errs multierror.Errors

	user, err := touint(fields[1])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.User = metrics.OptUintWith(user)

	nice, err := touint(fields[2])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Nice = metrics.OptUintWith(nice)

	sys, err := touint(fields[3])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Sys = metrics.OptUintWith(sys)

	idle, err := touint(fields[4])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Idle = metrics.OptUintWith(idle)

	wait, err := touint(fields[5])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Wait = metrics.OptUintWith(wait)

	irq, err := touint(fields[6])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Irq = metrics.OptUintWith(irq)

	softIrq, err := touint(fields[7])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.SoftIrq = metrics.OptUintWith(softIrq)

	stolen, err := touint(fields[8])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Stolen = metrics.OptUintWith(stolen)

	return cpuData, errs.Err()
}
