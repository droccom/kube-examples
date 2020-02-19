/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/golang/glog"
)

// OpsSchedule is the schedule of operations on attachments. It consists of a
// list `dt0,..,dtn` of time intervals in nanoseconds. The ith operation that
// must be performed must happen at `t0 + dti`, where t0 is an arbitrary start
// time for operations on attachments. "Arbitrary start time" means that the
// start time is not hardcoded in the schedule, and it is up to the schedule
// user to provide it. This is done because computing a schedule might take
// time; if the schedule was dependent on a t0, by the time it was computed
// `t0 + dti` might already have elapsed. Compute the schedule first, and then
// pick a t0.
//
// The memory footprint of OpsSchedule is theta(N) where N is the total number
// of operations (create/delete) that must be performed on the attachments. We
// are interested in large-scale ==> N can be large, this can be a problem.
// Or not: if a total of 120 millions of attachments were created by the driver,
// the schedule would roughly take a little bit more than 2.6 GiB.
type OpsSchedule []time.Duration

// Supported distributions of the operations on attachments.
const (
	steadyDistribution  = "steady"
	poissonDistribution = "poisson"
)

// newOpsSchedule returns a schedule for the operations on attachments. The
// returned schedule satisfies the `opsDistribution` and `opsPeriod` parameters.
func newOpsSchedule(opsDistribution string, opsPeriodSecs float64, totalOps uint64) OpsSchedule {
	opsSchedule := make(OpsSchedule, totalOps, totalOps)

	// For each operation on attachments, compute a dt such that the operation
	// should take place at `t0 + dt` where t0 is the arbitrary start time for
	// operations managed outside of this function.
	var dtFromStart time.Duration
	for i := uint64(0); i < totalOps; i++ {
		if opsDistribution == poissonDistribution {
			// The time in secs between an op and the next one is given by the
			// exponential distribution with rate `1/opsPeriodSecs`.
			dtFromPreviousOpSecs := opsPeriodSecs * rand.ExpFloat64()
			if dtFromPreviousOpSecs > 1000 {
				// Truncate dt from previous op because we only care about high
				// rates and we want to minimize the risk of overflowing
				// dtFromPreviousOpNanos.
				dtFromPreviousOpSecs = 1000
			}
			dtFromStart += time.Duration(float64(time.Second) * dtFromPreviousOpSecs)
		} else {
			// The ops on attachments happen with a constant period.
			dtFromStart += time.Duration(float64(time.Second) * opsPeriodSecs)
		}
		if dtFromStart < 0 {
			// Overflow! This should never happen, because at every iteration
			// dtNanos is less than or (approximately) equal to the time of the
			// whole throughput driver run, and an int64 is able to accommodate
			// runs that last longer than 20 years. We check anyway for safety.
			glog.Error("Encountered an overflow while computing timing of operations on attachments; lower number of attachments to create or increase ops rate to resolve.")
			os.Exit(100)
		}
		opsSchedule[i] = dtFromStart
	}

	return opsSchedule
}
