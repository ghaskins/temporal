// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package matching

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	clockspb "go.temporal.io/server/api/clock/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	commonclock "go.temporal.io/server/common/clock"
	hlc "go.temporal.io/server/common/clock/hybrid_logical_clock"
)

func mkNewSet(id string, clock clockspb.HybridLogicalClock) *persistencespb.CompatibleVersionSet {
	return &persistencespb.CompatibleVersionSet{
		SetIds:                 []string{hashBuildId(id)},
		BuildIds:               []*persistencespb.BuildId{{Id: id, State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock}},
		DefaultUpdateTimestamp: &clock,
	}
}

func mkInitialData(numSets int, clock clockspb.HybridLogicalClock) *persistencespb.VersioningData {
	sets := make([]*persistencespb.CompatibleVersionSet, numSets)
	for i := 0; i < numSets; i++ {
		sets[i] = mkNewSet(fmt.Sprintf("%v", i), clock)
	}
	return &persistencespb.VersioningData{
		VersionSets:            sets,
		DefaultUpdateTimestamp: &clock,
	}
}

func mkUserData(numSets int) *persistencespb.TaskQueueUserData {
	clock := hlc.Zero(1)
	return &persistencespb.TaskQueueUserData{
		Clock:          &clock,
		VersioningData: mkInitialData(numSets, clock),
	}
}

func mkNewDefReq(id string) *workflowservice.UpdateWorkerBuildIdCompatibilityRequest {
	return &workflowservice.UpdateWorkerBuildIdCompatibilityRequest{
		Operation: &workflowservice.UpdateWorkerBuildIdCompatibilityRequest_AddNewBuildIdInNewDefaultSet{
			AddNewBuildIdInNewDefaultSet: id,
		},
	}
}
func mkNewCompatReq(id, compat string, becomeDefault bool) *workflowservice.UpdateWorkerBuildIdCompatibilityRequest {
	return &workflowservice.UpdateWorkerBuildIdCompatibilityRequest{
		Operation: &workflowservice.UpdateWorkerBuildIdCompatibilityRequest_AddNewCompatibleBuildId{
			AddNewCompatibleBuildId: &workflowservice.UpdateWorkerBuildIdCompatibilityRequest_AddNewCompatibleVersion{
				NewBuildId:                id,
				ExistingCompatibleBuildId: compat,
				MakeSetDefault:            becomeDefault,
			},
		},
	}
}
func mkExistingDefault(id string) *workflowservice.UpdateWorkerBuildIdCompatibilityRequest {
	return &workflowservice.UpdateWorkerBuildIdCompatibilityRequest{
		Operation: &workflowservice.UpdateWorkerBuildIdCompatibilityRequest_PromoteSetByBuildId{
			PromoteSetByBuildId: id,
		},
	}
}
func mkPromoteInSet(id string) *workflowservice.UpdateWorkerBuildIdCompatibilityRequest {
	return &workflowservice.UpdateWorkerBuildIdCompatibilityRequest{
		Operation: &workflowservice.UpdateWorkerBuildIdCompatibilityRequest_PromoteBuildIdWithinSet{
			PromoteBuildIdWithinSet: id,
		},
	}
}
func mkMergeSet(primaryId string, secondaryId string) *workflowservice.UpdateWorkerBuildIdCompatibilityRequest {
	return &workflowservice.UpdateWorkerBuildIdCompatibilityRequest{
		Operation: &workflowservice.UpdateWorkerBuildIdCompatibilityRequest_MergeSets_{
			MergeSets: &workflowservice.UpdateWorkerBuildIdCompatibilityRequest_MergeSets{
				PrimarySetBuildId:   primaryId,
				SecondarySetBuildId: secondaryId,
			},
		},
	}
}

func TestNewDefaultUpdate(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	initialData := mkInitialData(2, clock)

	req := mkNewDefReq("2")
	nextClock := hlc.Next(clock, commonclock.NewRealTimeSource())
	updatedData, err := UpdateVersionSets(nextClock, initialData, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, mkInitialData(2, clock), initialData)

	expected := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &nextClock,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds:                 []string{hashBuildId("0")},
				BuildIds:               []*persistencespb.BuildId{{Id: "0", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock}},
				DefaultUpdateTimestamp: &clock,
			},
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock}},
				DefaultUpdateTimestamp: &clock,
			},
			{
				SetIds:                 []string{hashBuildId("2")},
				BuildIds:               []*persistencespb.BuildId{{Id: "2", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &nextClock}},
				DefaultUpdateTimestamp: &nextClock,
			},
		},
	}
	assert.Equal(t, expected, updatedData)

	asResp := ToBuildIdOrderingResponse(updatedData, 0)
	assert.Equal(t, "2", asResp.MajorVersionSets[2].BuildIds[0])
}

func TestNewDefaultSetUpdateOfEmptyData(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	initialData := mkInitialData(0, clock)

	req := mkNewDefReq("1")
	nextClock := hlc.Next(clock, commonclock.NewRealTimeSource())
	updatedData, err := UpdateVersionSets(nextClock, initialData, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, mkInitialData(0, clock), initialData)

	expected := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &nextClock,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &nextClock}},
				DefaultUpdateTimestamp: &nextClock,
			},
		},
	}
	assert.Equal(t, expected, updatedData)
}

func TestNewDefaultSetUpdateCompatWithCurDefault(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	initialData := mkInitialData(2, clock)

	req := mkNewCompatReq("1.1", "1", true)
	nextClock := hlc.Next(clock, commonclock.NewRealTimeSource())
	updatedData, err := UpdateVersionSets(nextClock, initialData, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, mkInitialData(2, clock), initialData)

	expected := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &nextClock,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds:                 []string{hashBuildId("0")},
				BuildIds:               []*persistencespb.BuildId{{Id: "0", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock}},
				DefaultUpdateTimestamp: &clock,
			},
			{
				SetIds: []string{hashBuildId("1")},
				BuildIds: []*persistencespb.BuildId{
					{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock},
					{Id: "1.1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &nextClock},
				},
				DefaultUpdateTimestamp: &nextClock,
			},
		},
	}
	assert.Equal(t, expected, updatedData)
}

func TestNewDefaultSetUpdateCompatWithNonDefaultSet(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	initialData := mkInitialData(2, clock)

	req := mkNewCompatReq("0.1", "0", true)
	nextClock := hlc.Next(clock, commonclock.NewRealTimeSource())
	updatedData, err := UpdateVersionSets(nextClock, initialData, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, mkInitialData(2, clock), initialData)

	expected := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &nextClock,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock}},
				DefaultUpdateTimestamp: &clock,
			},
			{
				SetIds: []string{hashBuildId("0")},
				BuildIds: []*persistencespb.BuildId{
					{Id: "0", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock},
					{Id: "0.1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &nextClock},
				},
				DefaultUpdateTimestamp: &nextClock,
			},
		},
	}
	assert.Equal(t, expected, updatedData)
}

func TestNewCompatibleWithVerInOlderSet(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	initialData := mkInitialData(2, clock)

	req := mkNewCompatReq("0.1", "0", false)
	nextClock := hlc.Next(clock, commonclock.NewRealTimeSource())
	updatedData, err := UpdateVersionSets(nextClock, initialData, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, mkInitialData(2, clock), initialData)

	expected := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &clock,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds: []string{hashBuildId("0")},
				BuildIds: []*persistencespb.BuildId{
					{Id: "0", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock},
					{Id: "0.1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &nextClock},
				},
				DefaultUpdateTimestamp: &nextClock,
			},
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock}},
				DefaultUpdateTimestamp: &clock,
			},
		},
	}

	assert.Equal(t, expected, updatedData)
	asResp := ToBuildIdOrderingResponse(updatedData, 0)
	assert.Equal(t, "0.1", asResp.MajorVersionSets[0].BuildIds[1])
}

func TestNewCompatibleWithNonDefaultSetUpdate(t *testing.T) {
	t.Parallel()
	clock0 := hlc.Zero(1)
	data := mkInitialData(2, clock0)

	req := mkNewCompatReq("0.1", "0", false)
	clock1 := hlc.Next(clock0, commonclock.NewRealTimeSource())
	data, err := UpdateVersionSets(clock1, data, req, 0, 0)
	assert.NoError(t, err)

	req = mkNewCompatReq("0.2", "0.1", false)
	clock2 := hlc.Next(clock1, commonclock.NewRealTimeSource())
	data, err = UpdateVersionSets(clock2, data, req, 0, 0)
	assert.NoError(t, err)

	expected := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &clock0,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds: []string{hashBuildId("0")},
				BuildIds: []*persistencespb.BuildId{
					{Id: "0", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0},
					{Id: "0.1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock1},
					{Id: "0.2", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock2},
				},
				DefaultUpdateTimestamp: &clock2,
			},
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0}},
				DefaultUpdateTimestamp: &clock0,
			},
		},
	}

	assert.Equal(t, expected, data)
	// Ensure setting a compatible version which targets a non-leaf compat version ends up without a branch
	req = mkNewCompatReq("0.3", "0.1", false)
	clock3 := hlc.Next(clock1, commonclock.NewRealTimeSource())
	data, err = UpdateVersionSets(clock3, data, req, 0, 0)
	assert.NoError(t, err)

	expected = &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &clock0,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds: []string{hashBuildId("0")},
				BuildIds: []*persistencespb.BuildId{
					{Id: "0", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0},
					{Id: "0.1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock1},
					{Id: "0.2", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock2},
					{Id: "0.3", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock3},
				},
				DefaultUpdateTimestamp: &clock3,
			},
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0}},
				DefaultUpdateTimestamp: &clock0,
			},
		},
	}

	assert.Equal(t, expected, data)
}

func TestCompatibleTargetsNotFound(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	data := mkInitialData(1, clock)

	req := mkNewCompatReq("1.1", "1", false)
	nextClock := hlc.Next(clock, commonclock.NewRealTimeSource())
	_, err := UpdateVersionSets(nextClock, data, req, 0, 0)
	var notFound *serviceerror.NotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestMakeExistingSetDefault(t *testing.T) {
	t.Parallel()
	clock0 := hlc.Zero(1)
	data := mkInitialData(3, clock0)

	req := mkExistingDefault("1")
	clock1 := hlc.Next(clock0, commonclock.NewRealTimeSource())
	data, err := UpdateVersionSets(clock1, data, req, 0, 0)
	assert.NoError(t, err)

	expected := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &clock1,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds: []string{hashBuildId("0")},
				BuildIds: []*persistencespb.BuildId{
					{Id: "0", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0},
				},
				DefaultUpdateTimestamp: &clock0,
			},
			{
				SetIds:                 []string{hashBuildId("2")},
				BuildIds:               []*persistencespb.BuildId{{Id: "2", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0}},
				DefaultUpdateTimestamp: &clock0,
			},
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0}},
				DefaultUpdateTimestamp: &clock0,
			},
		},
	}

	assert.Equal(t, expected, data)
	// Add a compatible version to a set and then make that set the default via the compatible version
	req = mkNewCompatReq("0.1", "0", true)

	clock2 := hlc.Next(clock1, commonclock.NewRealTimeSource())
	data, err = UpdateVersionSets(clock2, data, req, 0, 0)
	assert.NoError(t, err)

	expected = &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &clock2,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds:                 []string{hashBuildId("2")},
				BuildIds:               []*persistencespb.BuildId{{Id: "2", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0}},
				DefaultUpdateTimestamp: &clock0,
			},
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0}},
				DefaultUpdateTimestamp: &clock0,
			},
			{
				SetIds: []string{hashBuildId("0")},
				BuildIds: []*persistencespb.BuildId{
					{Id: "0", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0},
					{Id: "0.1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock2},
				},
				DefaultUpdateTimestamp: &clock2,
			},
		},
	}
	assert.Equal(t, expected, data)
}

func TestSayVersionIsCompatWithDifferentSetThanItsAlreadyCompatWithNotAllowed(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	data := mkInitialData(3, clock)

	req := mkNewCompatReq("0.1", "0", false)
	data, err := UpdateVersionSets(clock, data, req, 0, 0)
	assert.NoError(t, err)

	req = mkNewCompatReq("0.1", "1", false)
	_, err = UpdateVersionSets(clock, data, req, 0, 0)
	var invalidArgument *serviceerror.InvalidArgument
	assert.ErrorAs(t, err, &invalidArgument)
}

func TestLimitsMaxSets(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	maxSets := 10
	data := mkInitialData(maxSets, clock)

	req := mkNewDefReq("10")
	_, err := UpdateVersionSets(clock, data, req, maxSets, 0)
	var failedPrecondition *serviceerror.FailedPrecondition
	assert.ErrorAs(t, err, &failedPrecondition)
}

func TestLimitsMaxBuildIds(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	maxBuildIds := 10
	data := mkInitialData(maxBuildIds, clock)

	req := mkNewDefReq("10")
	_, err := UpdateVersionSets(clock, data, req, 0, maxBuildIds)
	var failedPrecondition *serviceerror.FailedPrecondition
	assert.ErrorAs(t, err, &failedPrecondition)
}

func TestPromoteWithinVersion(t *testing.T) {
	t.Parallel()
	clock0 := hlc.Zero(1)
	data := mkInitialData(2, clock0)

	req := mkNewCompatReq("0.1", "0", false)
	clock1 := hlc.Next(clock0, commonclock.NewRealTimeSource())
	data, err := UpdateVersionSets(clock1, data, req, 0, 0)
	assert.NoError(t, err)
	req = mkNewCompatReq("0.2", "0", false)
	clock2 := hlc.Next(clock1, commonclock.NewRealTimeSource())
	data, err = UpdateVersionSets(clock2, data, req, 0, 0)
	assert.NoError(t, err)
	req = mkPromoteInSet("0.1")
	clock3 := hlc.Next(clock2, commonclock.NewRealTimeSource())
	data, err = UpdateVersionSets(clock3, data, req, 0, 0)
	assert.NoError(t, err)

	expected := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &clock0,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds: []string{hashBuildId("0")},
				BuildIds: []*persistencespb.BuildId{
					{Id: "0", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0},
					{Id: "0.2", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock2},
					{Id: "0.1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock1},
				},
				DefaultUpdateTimestamp: &clock3,
			},
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock0}},
				DefaultUpdateTimestamp: &clock0,
			},
		},
	}
	assert.Equal(t, expected, data)
}

func TestAddNewDefaultAlreadyExtantVersionWithNoConflictSucceeds(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	original := mkInitialData(3, clock)

	req := mkNewDefReq("2")
	updated, err := UpdateVersionSets(clock, original, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, original, updated)
}

func TestAddToExistingSetAlreadyExtantVersionWithNoConflictSucceeds(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	req := mkNewCompatReq("1.1", "1", false)
	original, err := UpdateVersionSets(clock, mkInitialData(3, clock), req, 0, 0)
	assert.NoError(t, err)
	updated, err := UpdateVersionSets(clock, original, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, original, updated)
}

func TestAddToExistingSetAlreadyExtantVersionErrorsIfNotDefault(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	req := mkNewCompatReq("1.1", "1", true)
	original, err := UpdateVersionSets(clock, mkInitialData(3, clock), req, 0, 0)
	assert.NoError(t, err)
	req = mkNewCompatReq("1", "1.1", true)
	_, err = UpdateVersionSets(clock, original, req, 0, 0)
	var invalidArgument *serviceerror.InvalidArgument
	assert.ErrorAs(t, err, &invalidArgument)
}

func TestAddToExistingSetAlreadyExtantVersionErrorsIfNotDefaultSet(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	req := mkNewCompatReq("1.1", "1", false)
	original, err := UpdateVersionSets(clock, mkInitialData(3, clock), req, 0, 0)
	assert.NoError(t, err)
	req = mkNewCompatReq("1.1", "1", true)
	_, err = UpdateVersionSets(clock, original, req, 0, 0)
	var invalidArgument *serviceerror.InvalidArgument
	assert.ErrorAs(t, err, &invalidArgument)
}

func TestPromoteWithinSetAlreadyPromotedIsANoop(t *testing.T) {
	t.Parallel()
	clock0 := hlc.Zero(1)
	original := mkInitialData(3, clock0)
	req := mkPromoteInSet("1")
	clock1 := hlc.Zero(2)
	updated, err := UpdateVersionSets(clock1, original, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, original, updated)
}

func TestPromoteSetAlreadyPromotedIsANoop(t *testing.T) {
	t.Parallel()
	clock0 := hlc.Zero(1)
	original := mkInitialData(3, clock0)
	req := mkExistingDefault("2")
	clock1 := hlc.Zero(2)
	updated, err := UpdateVersionSets(clock1, original, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, original, updated)
}

func TestAddAlreadyExtantVersionAsDefaultErrors(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	data := mkInitialData(3, clock)

	req := mkNewDefReq("0")
	_, err := UpdateVersionSets(clock, data, req, 0, 0)
	var invalidArgument *serviceerror.InvalidArgument
	assert.ErrorAs(t, err, &invalidArgument)
}

func TestAddAlreadyExtantVersionToAnotherSetErrors(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	data := mkInitialData(3, clock)

	req := mkNewCompatReq("0", "1", false)
	_, err := UpdateVersionSets(clock, data, req, 0, 0)
	var invalidArgument *serviceerror.InvalidArgument
	assert.ErrorAs(t, err, &invalidArgument)
}

func TestMakeSetDefaultTargetingNonexistentVersionErrors(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	data := mkInitialData(3, clock)

	req := mkExistingDefault("crab boi")
	_, err := UpdateVersionSets(clock, data, req, 0, 0)
	var notFound *serviceerror.NotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestPromoteWithinSetTargetingNonexistentVersionErrors(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	data := mkInitialData(3, clock)

	req := mkPromoteInSet("i'd rather be writing rust ;)")
	_, err := UpdateVersionSets(clock, data, req, 0, 0)
	var notFound *serviceerror.NotFound
	assert.ErrorAs(t, err, &notFound)
}

func TestToBuildIdOrderingResponseTrimsResponse(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	data := mkInitialData(3, clock)
	actual := ToBuildIdOrderingResponse(data, 2)
	expected := []*taskqueuepb.CompatibleVersionSet{{BuildIds: []string{"1"}}, {BuildIds: []string{"2"}}}
	assert.Equal(t, expected, actual.MajorVersionSets)
}

func TestToBuildIdOrderingResponseOmitsDeleted(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	data := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &clock,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds: []string{hashBuildId("0")},
				BuildIds: []*persistencespb.BuildId{
					{Id: "0", State: persistencespb.STATE_DELETED, StateUpdateTimestamp: &clock},
					{Id: "0.1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock},
				},
				DefaultUpdateTimestamp: &clock,
			},
		},
	}
	actual := ToBuildIdOrderingResponse(data, 0)
	expected := []*taskqueuepb.CompatibleVersionSet{{BuildIds: []string{"0.1"}}}
	assert.Equal(t, expected, actual.MajorVersionSets)
}

func TestHashBuildId(t *testing.T) {
	t.Parallel()
	// This function should never change.
	assert.Equal(t, "ftrPuUeORv2JD4Wp2wTU", hashBuildId("my-build-id"))
}

func TestGetBuildIdDeltas(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(0)
	prev := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &clock,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds:                 []string{hashBuildId("0")},
				BuildIds:               []*persistencespb.BuildId{{Id: "0", State: persistencespb.STATE_DELETED, StateUpdateTimestamp: &clock}, {Id: "0.1", State: persistencespb.STATE_ACTIVE}},
				DefaultUpdateTimestamp: &clock,
			},
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock}},
				DefaultUpdateTimestamp: &clock,
			},
		},
	}
	curr := &persistencespb.VersioningData{
		DefaultUpdateTimestamp: &clock,
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds:                 []string{hashBuildId("0")},
				BuildIds:               []*persistencespb.BuildId{{Id: "0.1", State: persistencespb.STATE_ACTIVE}},
				DefaultUpdateTimestamp: &clock,
			},
			{
				SetIds:                 []string{hashBuildId("1")},
				BuildIds:               []*persistencespb.BuildId{{Id: "1", State: persistencespb.STATE_DELETED, StateUpdateTimestamp: &clock}, {Id: "1.1", State: persistencespb.STATE_ACTIVE, StateUpdateTimestamp: &clock}},
				DefaultUpdateTimestamp: &clock,
			},
		},
	}
	added, removed := GetBuildIdDeltas(prev, curr)
	assert.Equal(t, []string{"1"}, removed)
	assert.Equal(t, []string{"1.1"}, added)
}

func TestGetBuildIdDeltas_AcceptsNils(t *testing.T) {
	t.Parallel()
	added, removed := GetBuildIdDeltas(nil, nil)
	assert.Equal(t, []string(nil), removed)
	assert.Equal(t, []string(nil), added)
}

func Test_RemoveBuildIds_PutsTombstonesOnSuppliedBuildIds(t *testing.T) {
	t.Parallel()
	c0 := hlc.Zero(0)
	data := mkInitialData(3, c0)
	c1 := c0
	c1.Version++

	expected := &persistencespb.VersioningData{
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds: []string{hashBuildId("0")},
				BuildIds: []*persistencespb.BuildId{
					{
						Id:                   "0",
						State:                persistencespb.STATE_DELETED,
						StateUpdateTimestamp: &c1,
					},
				},
				DefaultUpdateTimestamp: &c0,
			},
			{
				SetIds: []string{hashBuildId("1")},
				BuildIds: []*persistencespb.BuildId{
					{
						Id:                   "1",
						State:                persistencespb.STATE_DELETED,
						StateUpdateTimestamp: &c1,
					},
				},
				DefaultUpdateTimestamp: &c0,
			},
			{
				SetIds: []string{hashBuildId("2")},
				BuildIds: []*persistencespb.BuildId{
					{
						Id:                   "2",
						State:                persistencespb.STATE_ACTIVE,
						StateUpdateTimestamp: &c0,
					},
				},
				DefaultUpdateTimestamp: &c0,
			},
		},
		DefaultUpdateTimestamp: &c0,
	}

	actual := RemoveBuildIds(c1, data, []string{"0", "1"})
	assert.Equal(t, expected, actual)
	// Method does not mutate original data
	assert.Equal(t, mkInitialData(3, c0), data)
}

func Test_ClearTombstones(t *testing.T) {
	t.Parallel()
	c0 := hlc.Zero(0)

	makeData := func() *persistencespb.VersioningData {
		return &persistencespb.VersioningData{
			VersionSets: []*persistencespb.CompatibleVersionSet{
				{
					SetIds: []string{hashBuildId("0")},
					BuildIds: []*persistencespb.BuildId{
						{
							Id:                   "0",
							State:                persistencespb.STATE_DELETED,
							StateUpdateTimestamp: &c0,
						},
					},
					DefaultUpdateTimestamp: &c0,
				},
				{
					SetIds: []string{hashBuildId("1")},
					BuildIds: []*persistencespb.BuildId{
						{
							Id:                   "1",
							State:                persistencespb.STATE_DELETED,
							StateUpdateTimestamp: &c0,
						},
						{
							Id:                   "1.1",
							State:                persistencespb.STATE_ACTIVE,
							StateUpdateTimestamp: &c0,
						},
					},
					DefaultUpdateTimestamp: &c0,
				},
				{
					SetIds: []string{hashBuildId("2")},
					BuildIds: []*persistencespb.BuildId{
						{
							Id:                   "2",
							State:                persistencespb.STATE_ACTIVE,
							StateUpdateTimestamp: &c0,
						},
					},
					DefaultUpdateTimestamp: &c0,
				},
			},
			DefaultUpdateTimestamp: &c0,
		}
	}
	expected := &persistencespb.VersioningData{
		VersionSets: []*persistencespb.CompatibleVersionSet{
			{
				SetIds: []string{hashBuildId("1")},
				BuildIds: []*persistencespb.BuildId{
					{
						Id:                   "1.1",
						State:                persistencespb.STATE_ACTIVE,
						StateUpdateTimestamp: &c0,
					},
				},
				DefaultUpdateTimestamp: &c0,
			},
			{
				SetIds: []string{hashBuildId("2")},
				BuildIds: []*persistencespb.BuildId{
					{
						Id:                   "2",
						State:                persistencespb.STATE_ACTIVE,
						StateUpdateTimestamp: &c0,
					},
				},
				DefaultUpdateTimestamp: &c0,
			},
		},
		DefaultUpdateTimestamp: &c0,
	}
	original := makeData()
	actual := ClearTombstones(original)
	assert.Equal(t, expected, actual)
	// Method does not mutate original data
	assert.Equal(t, makeData(), original)
}

func TestMergeSets(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	initialData := mkInitialData(4, clock)

	req := mkMergeSet("1", "2")
	nextClock := hlc.Next(clock, commonclock.NewRealTimeSource())
	updatedData, err := UpdateVersionSets(nextClock, initialData, req, 0, 0)
	assert.NoError(t, err)
	// Should only be three sets now
	assert.Equal(t, 3, len(updatedData.VersionSets))
	// The overall default set should not have changed
	assert.Equal(t, "3", updatedData.GetVersionSets()[2].GetBuildIds()[0].Id)
	// But set 1 should now have 2, maintaining 1 as the default ID
	assert.Equal(t, "1", updatedData.GetVersionSets()[1].GetBuildIds()[1].Id)
	assert.Equal(t, "2", updatedData.GetVersionSets()[1].GetBuildIds()[0].Id)
	// Ensure it has the set ids of both sets
	bothSetIds := mergeSetIDs([]string{hashBuildId("1")}, []string{hashBuildId("2")})
	assert.Equal(t, bothSetIds, updatedData.GetVersionSets()[1].GetSetIds())
	assert.Equal(t, initialData.DefaultUpdateTimestamp, updatedData.DefaultUpdateTimestamp)
	assert.Equal(t, nextClock, *updatedData.GetVersionSets()[1].DefaultUpdateTimestamp)
	// Initial data should not have changed
	assert.Equal(t, 4, len(initialData.VersionSets))
	for _, set := range initialData.VersionSets {
		assert.Equal(t, 1, len(set.GetSetIds()))
		assert.Equal(t, clock, *set.DefaultUpdateTimestamp)
	}

	// Same merge request must be idempotent
	nextClock2 := hlc.Next(nextClock, commonclock.NewRealTimeSource())
	updatedData2, err := UpdateVersionSets(nextClock2, updatedData, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(updatedData2.VersionSets))
	assert.Equal(t, "3", updatedData2.GetVersionSets()[2].GetBuildIds()[0].Id)
	assert.Equal(t, "1", updatedData2.GetVersionSets()[1].GetBuildIds()[1].Id)
	assert.Equal(t, "2", updatedData2.GetVersionSets()[1].GetBuildIds()[0].Id)
	assert.Equal(t, initialData.DefaultUpdateTimestamp, updatedData2.DefaultUpdateTimestamp)
	// Clock shouldn't have changed
	assert.Equal(t, nextClock, *updatedData2.GetVersionSets()[1].DefaultUpdateTimestamp)

	// Verify merging into the current default maintains that set as the default
	req = mkMergeSet("3", "0")
	nextClock3 := hlc.Next(nextClock2, commonclock.NewRealTimeSource())
	updatedData3, err := UpdateVersionSets(nextClock3, updatedData2, req, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(updatedData3.VersionSets))
	assert.Equal(t, "3", updatedData3.GetVersionSets()[1].GetBuildIds()[1].Id)
	assert.Equal(t, "0", updatedData3.GetVersionSets()[1].GetBuildIds()[0].Id)
	assert.Equal(t, "1", updatedData3.GetVersionSets()[0].GetBuildIds()[1].Id)
	assert.Equal(t, "2", updatedData3.GetVersionSets()[0].GetBuildIds()[0].Id)
	assert.Equal(t, initialData.DefaultUpdateTimestamp, updatedData3.DefaultUpdateTimestamp)
	assert.Equal(t, nextClock3, *updatedData3.GetVersionSets()[1].DefaultUpdateTimestamp)
}

func TestMergeInvalidTargets(t *testing.T) {
	t.Parallel()
	clock := hlc.Zero(1)
	initialData := mkInitialData(4, clock)

	nextClock := hlc.Next(clock, commonclock.NewRealTimeSource())
	req := mkMergeSet("lol", "2")
	_, err := UpdateVersionSets(nextClock, initialData, req, 0, 0)
	assert.Error(t, err)

	req2 := mkMergeSet("2", "nope")
	_, err2 := UpdateVersionSets(nextClock, initialData, req2, 0, 0)
	assert.Error(t, err2)
}
