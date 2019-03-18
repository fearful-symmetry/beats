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

//+build darwin freebsd linux openbsd

package mdinfo

//MockData is a fake type for testing
type MockData string

//Close closes the connection to the /dev/md device
func (dev MockData) Close() error {
	return nil
}

//GetArrayInfo returns a struct describing the state of the RAID array.
func (dev MockData) GetArrayInfo() (MDArrayInfo, error) {

	dat := MDArrayInfo{
		MajorVersion:  1,
		MinorVersion:  2,
		Ctime:         0,
		Level:         0,
		RAIDDisks:     1,
		NrDisks:       1,
		NotPersistent: 0,
		Utime:         0,
		State:         1,
		ActiveDisks:   1,
		WorkingDisks:  1,
		FailedDisks:   0,
		SpareDisks:    0,
		ChunkSize:     0,
	}
	return dat, nil
}
