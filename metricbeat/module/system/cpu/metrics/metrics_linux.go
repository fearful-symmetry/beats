package metrics

import (
	"bufio"
	"strings"

	"github.com/elastic/beats/v7/libbeat/common"
	"github.com/joeshaw/multierror"
	"github.com/pkg/errors"
)

func (self cpuMetrics) Total() uint64 {
	return self.totals.User + self.totals.Nice + self.totals.Sys + self.totals.Idle +
		self.totals.Wait + self.totals.Irq + self.totals.SoftIrq + self.totals.Stolen
}

func (self cpuMetrics) FillTicks(event *common.MapStr) {

	event.Put("user.ticks", self.totals.User)
	event.Put("system.ticks", self.totals.Sys)
	event.Put("idle.ticks", self.totals.Idle)
	event.Put("nice.ticks", self.totals.Nice)
	event.Put("irq.ticks", self.totals.Irq)
	event.Put("iowait.ticks", self.totals.Wait)
	event.Put("softirq.ticks", self.totals.SoftIrq)
	event.Put("steal.ticks", self.totals.Stolen)

}

func fillCPUMetrics(event *common.MapStr, current, prev cpuMetrics, numCPU int, timeDelta uint64, pathPostfix string) {
	// IOWait time is excluded from the total as per #7627.
	idleTime := cpuMetricTimeDelta(prev.totals.Idle, current.totals.Idle, timeDelta, numCPU) + cpuMetricTimeDelta(prev.totals.Wait, current.totals.Wait, timeDelta, numCPU)
	totalPct := common.Round(float64(numCPU)-idleTime, common.DefaultDecimalPlacesCount)

	event.Put("total"+pathPostfix, totalPct)
	event.Put("user"+pathPostfix, cpuMetricTimeDelta(prev.totals.User, current.totals.User, timeDelta, numCPU))
	event.Put("system"+pathPostfix, cpuMetricTimeDelta(prev.totals.Sys, current.totals.Sys, timeDelta, numCPU))
	event.Put("idle"+pathPostfix, cpuMetricTimeDelta(prev.totals.Idle, current.totals.Idle, timeDelta, numCPU))
	event.Put("nice"+pathPostfix, cpuMetricTimeDelta(prev.totals.Nice, current.totals.Nice, timeDelta, numCPU))
	event.Put("irq"+pathPostfix, cpuMetricTimeDelta(prev.totals.Irq, current.totals.Irq, timeDelta, numCPU))
	event.Put("softirq"+pathPostfix, cpuMetricTimeDelta(prev.totals.SoftIrq, current.totals.SoftIrq, timeDelta, numCPU))
	event.Put("iowait"+pathPostfix, cpuMetricTimeDelta(prev.totals.Wait, current.totals.Wait, timeDelta, numCPU))
	event.Put("steal"+pathPostfix, cpuMetricTimeDelta(prev.totals.Stolen, current.totals.Stolen, timeDelta, numCPU))
}

func scanStatFile(scanner *bufio.Scanner) (MetricMap, error) {
	cpuData, err := statScanner(scanner, parseCPULine)
	if err != nil {
		return nil, errors.Wrap(err, "error scanning stat file")
	}
	return cpuData, nil
}

func parseCPULine(line string) (cpu, error) {
	cpuData := cpu{}
	fields := strings.Fields(line)
	var errs multierror.Errors
	var err error

	cpuData.User, err = touint(fields[1])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Nice, err = touint(fields[2])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Sys, err = touint(fields[3])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Idle, err = touint(fields[4])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Wait, err = touint(fields[5])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Irq, err = touint(fields[6])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.SoftIrq, err = touint(fields[7])
	if err != nil {
		errs = append(errs, err)
	}
	cpuData.Stolen, err = touint(fields[8])
	if err != nil {
		errs = append(errs, err)
	}
	return cpuData, errs.Err()
}
