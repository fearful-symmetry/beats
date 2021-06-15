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

package metrics

// Uint

// OptUint is a wrapper for "optional" types, with the bool value indicating
// if the stored int is a legitimate value.
type OptUint struct {
	exists bool
	value  uint64
}

// NewUintNone returns a new OptUint wrapper
func NewUintNone() OptUint {
	return OptUint{
		exists: false,
		value:  0,
	}
}

// OptUintWith returns a new OptUint wrapper with a given int
func OptUintWith(i uint64) OptUint {
	return OptUint{
		exists: true,
		value:  i,
	}
}

// None marks the Uint as not having a value.
func (opt *OptUint) None() {
	opt.exists = false
}

// Exists returns true if the underlying value is valid
func (opt OptUint) Exists() bool {
	return opt.exists
}

// IsNone returns true if the value exists
func (opt OptUint) IsNone(i uint64) {
	opt.value = i
	opt.exists = true
}

// Value returns true if the value exists
func (opt OptUint) Value() (uint64, bool) {
	return opt.value, opt.exists
}

// IsZero returns true if the underlying value nil
func (opt OptUint) IsZero() bool {
	return !opt.exists
}

// ValueOr returns the stored value, or a given int
// Please do not use this for populating reported data,
// as we actually want to avoid sending zeros where values are functionally null
func (opt OptUint) ValueOr(i uint64) uint64 {
	if opt.exists {
		return opt.value
	}
	return i
}

// SumOptUint sums a list of OptUint values
func SumOptUint(opts ...OptUint) uint64 {
	var sum uint64
	for _, opt := range opts {
		sum += opt.ValueOr(0)
	}
	return sum
}

// Float

// OptFloat is a wrapper for "optional" types, with the bool value indicating
// if the stored int is a legitimate value.
type OptFloat struct {
	exists bool
	value  float64
}

// NewFloatNone returns a new uint wrapper
func NewFloatNone() OptFloat {
	return OptFloat{
		exists: false,
		value:  0,
	}
}

// OptFloatWith returns a new uint wrapper for the specified value
func OptFloatWith(f float64) OptFloat {
	return OptFloat{
		exists: true,
		value:  f,
	}
}

// Exists returns true if the underlying value is valid
func (opt OptFloat) Exists() bool {
	return opt.exists
}

// ValueOrZero returns the stored value, or zero
// Please do not use this for populating reported data,
// as we actually want to avoid sending zeros where values are functionally null
func (opt OptFloat) ValueOrZero() float64 {
	if opt.exists {
		return opt.value
	}
	return 0
}
