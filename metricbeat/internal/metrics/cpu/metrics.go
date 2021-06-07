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
	"github.com/pkg/errors"

	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/elastic/beats/v7/metricbeat/internal/metrics"
)

// CPU manages the CPU metrics from /proc/stat
// If a given metric isn't available on a given platform,
// The value will be null. All methods that use these fields
// should assume that any value can be null.
// The values are in "ticks", which translates to milliseconds of CPU time
type CPU struct {
	User    metrics.OptUint `struct:"user"`
	Sys     metrics.OptUint `struct:"system"`
	Idle    metrics.OptUint `struct:"idle"`
	Nice    metrics.OptUint `struct:"nice"`    // Linux, Darwin, BSD
	Irq     metrics.OptUint `struct:"irq"`     // Linux and openbsd
	Wait    metrics.OptUint `struct:"iowait"`  // Linux and AIX
	SoftIrq metrics.OptUint `struct:"softirq"` // Linux only
	Stolen  metrics.OptUint `struct:"steal"`   // Linux only
}

// MetricOpts defines the fields that are passed along to the formatted output
type MetricOpts struct {
	Ticks                 bool
	Percentages           bool
	NormalizedPercentages bool
}

// CPUMetrics carries global and per-core CPU metrics
type CPUMetrics struct {
	totals CPU
	// list carries the same data, broken down by CPU
	list []CPU
}

// Total returns the total CPU time in ticks as scraped by the API
func (cpu CPU) Total() uint64 {
	// it's generally safe to blindly sum these up,
	// As we're just trying to get a total of all CPU time.
	return cpu.User.ValueOrZero() + cpu.Nice.ValueOrZero() +
		cpu.Sys.ValueOrZero() + cpu.Idle.ValueOrZero() +
		cpu.Wait.ValueOrZero() + cpu.Irq.ValueOrZero() +
		cpu.SoftIrq.ValueOrZero() + cpu.Stolen.ValueOrZero()

}

/*
The below code implements a "metrics tracker" that gives us the ability to
calculate CPU percentages, as we average usage across a time period.
*/

// Monitor is used to monitor the overall CPU usage of the system over time.
type Monitor struct {
	lastSample CPUMetrics
	Hostfs     string
}

// New returns a new CPU metrics monitor
// Hostfs is only relevant on linux and freebsd.
func New(hostfs string) *Monitor {
	return &Monitor{Hostfs: hostfs}
}

// Fetch collects a new sample of the CPU usage metrics.
// This will overwrite the currently stored samples.
func (m *Monitor) Fetch() (Metrics, error) {
	metric, err := Get(m.Hostfs)
	if err != nil {
		return Metrics{}, errors.Wrap(err, "Error fetching CPU metrics")
	}

	oldLastSample := m.lastSample
	m.lastSample = metric

	return Metrics{previousSample: oldLastSample.totals, currentSample: metric.totals, count: len(metric.list), isTotals: true}, nil
}

// FetchCores collects a new sample of CPU usage metrics per-core
// This will overwrite the currently stored samples.
func (m *Monitor) FetchCores() ([]Metrics, error) {

	metric, err := Get(m.Hostfs)
	if err != nil {
		return nil, errors.Wrap(err, "Error fetching CPU metrics")
	}

	coreMetrics := make([]Metrics, len(metric.list))
	for i := 0; i < len(metric.list); i++ {
		lastMetric := CPU{}
		// Count of CPUs can change
		if len(m.lastSample.list) > i {
			lastMetric = m.lastSample.list[i]
		}
		coreMetrics[i] = Metrics{
			currentSample:  metric.list[i],
			previousSample: lastMetric,
			isTotals:       false,
		}
	}
	m.lastSample = metric
	return coreMetrics, nil
}

// Metrics stores the current and the last sample collected by a Beat.
type Metrics struct {
	previousSample CPU
	currentSample  CPU
	count          int
	isTotals       bool
}

// Format returns the final MapStr data object for the metrics.
func (metrics Metrics) Format(opts MetricOpts) (common.MapStr, error) {

	timeDelta := metrics.currentSample.Total() - metrics.previousSample.Total()
	if timeDelta <= 0 {
		return nil, errors.New("Previous sample is newer than current sample")
	}
	normCPU := metrics.count
	if !metrics.isTotals {
		normCPU = 1
	}

	// In the future we might want to do this differently, but for now the `if` statements are more reliable than lots of reflection.
	formattedMetrics := common.MapStr{}
	if opts.Percentages {
		formattedMetrics.Put("total.pct", createTotal(metrics.previousSample, metrics.currentSample, timeDelta, normCPU))
	}
	if opts.NormalizedPercentages {
		formattedMetrics.Put("total.norm.pct", createTotal(metrics.previousSample, metrics.currentSample, timeDelta, 1))
	}

	if metrics.currentSample.User.Exists() {
		formattedMetrics["user"] = fillMetric(opts, metrics.currentSample.User, metrics.previousSample.User, timeDelta, normCPU)
	}
	if metrics.currentSample.Sys.Exists() {
		formattedMetrics["system"] = fillMetric(opts, metrics.currentSample.Sys, metrics.previousSample.Sys, timeDelta, normCPU)
	}
	if metrics.currentSample.Idle.Exists() {
		formattedMetrics["idle"] = fillMetric(opts, metrics.currentSample.Idle, metrics.previousSample.Idle, timeDelta, normCPU)
	}
	if metrics.currentSample.Nice.Exists() {
		formattedMetrics["nice"] = fillMetric(opts, metrics.currentSample.Nice, metrics.previousSample.Nice, timeDelta, normCPU)
	}
	if metrics.currentSample.Irq.Exists() {
		formattedMetrics["irq"] = fillMetric(opts, metrics.currentSample.Irq, metrics.previousSample.Irq, timeDelta, normCPU)
	}
	if metrics.currentSample.Wait.Exists() {
		formattedMetrics["iowait"] = fillMetric(opts, metrics.currentSample.Wait, metrics.previousSample.Wait, timeDelta, normCPU)
	}
	if metrics.currentSample.SoftIrq.Exists() {
		formattedMetrics["softirq"] = fillMetric(opts, metrics.currentSample.SoftIrq, metrics.previousSample.SoftIrq, timeDelta, normCPU)
	}
	if metrics.currentSample.Stolen.Exists() {
		formattedMetrics["steal"] = fillMetric(opts, metrics.currentSample.Stolen, metrics.previousSample.Stolen, timeDelta, normCPU)
	}

	return formattedMetrics, nil
}

func createTotal(prev, cur CPU, timeDelta uint64, numCPU int) float64 {
	idleTime := cpuMetricTimeDelta(prev.Idle, cur.Idle, timeDelta, numCPU)
	// Subtract wait time from total
	// Wait time is not counted from the total as per #7627.
	if cur.Wait.Exists() {
		idleTime = idleTime + cpuMetricTimeDelta(prev.Wait, cur.Wait, timeDelta, numCPU)
	}
	return common.Round(float64(numCPU)-idleTime, common.DefaultDecimalPlacesCount)
}

func fillMetric(opts MetricOpts, cur, prev metrics.OptUint, timeDelta uint64, numCPU int) common.MapStr {
	event := common.MapStr{}
	if opts.Ticks {
		event.Put("ticks", cur.ValueOrZero())
	}
	if opts.Percentages {
		event.Put("pct", cpuMetricTimeDelta(prev, cur, timeDelta, numCPU))
	}
	if opts.NormalizedPercentages {
		event.Put("norm.pct", cpuMetricTimeDelta(prev, cur, timeDelta, 1))
	}

	return event
}

// CPUCount returns the count of CPUs. When available, use this instead of runtime.NumCPU()
func (m *Metrics) CPUCount() int {
	return m.count
}

// cpuMetricTimeDelta is a helper used by fillTicks to calculate the delta between two CPU tick values
func cpuMetricTimeDelta(prev, current metrics.OptUint, timeDelta uint64, numCPU int) float64 {
	cpuDelta := int64(current.ValueOrZero() - prev.ValueOrZero())
	pct := float64(cpuDelta) / float64(timeDelta)
	return common.Round(pct*float64(numCPU), common.DefaultDecimalPlacesCount)
}
