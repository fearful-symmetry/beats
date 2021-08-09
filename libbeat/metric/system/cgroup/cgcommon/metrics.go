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

package cgcommon

import (
	"bufio"
	"fmt"
	"os"

	"github.com/pkg/errors"

	"github.com/elastic/beats/v7/libbeat/opt"
)

// CPUUsage wraps the CPU usage time values for the CPU controller metrics
type CPUUsage struct {
	NS   uint64     `json:"ns" struct:"ns"`
	Pct  opt.Float  `json:"pct" struct:"pct"`
	Norm opt.PctOpt `json:"norm" struct:"norm"`
}

// Pressure contains load metrics for a controller,
// Broken apart into 10, 60, and 300 second samples,
// as well as a total time in US
type Pressure struct {
	Ten          opt.Pct `json:"10" struct:"10"`
	Sixty        opt.Pct `json:"60" struct:"60"`
	ThreeHundred opt.Pct `json:"300" struct:"300"`
	Total        uint64  `json:"total" struct:"total"`
}

// GetPressure takes the path of a *.pressure file and returns a
// map of the pressure (IO contension) stats for the cgroup
// on CPU controllers, the only key will be "some"
// on IO controllers, the keys will be "some" and "full"
// See https://github.com/torvalds/linux/blob/master/Documentation/accounting/psi.rst
func GetPressure(path string) (map[string]Pressure, error) {
	pressureData := make(map[string]Pressure)

	f, err := os.Open(path)
	if err != nil {
		return pressureData, errors.Wrap(err, "error reading cpu.stat")
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var stallTime string
		data := Pressure{}
		matched, err := fmt.Sscanf(sc.Text(), "%s avg10=%f avg60=%f avg300=%f total=%d", &stallTime, &data.Ten.Pct, &data.Sixty.Pct, &data.ThreeHundred.Pct, &data.Total)
		if err != nil {
			return pressureData, errors.Wrapf(err, "error scanning file: %s", path)
		}
		// Assume that if we didn't match at least three numbers, something has gone wrong
		if matched < 3 {
			return pressureData, fmt.Errorf("Error: only matched %d fields from file %s", matched, path)
		}

		pressureData[stallTime] = data

	}

	return pressureData, nil
}
