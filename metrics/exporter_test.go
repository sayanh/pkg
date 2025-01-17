/*
Copyright 2018 The Knative Authors.
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
package metrics

import (
	"os"
	"path"
	"testing"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	. "knative.dev/pkg/logging/testing"
	"knative.dev/pkg/metrics/metricskey"
)

// TODO UTs should move to eventing and serving, as appropriate.
// 	See https://github.com/knative/pkg/issues/608

const (
	testNS            = "test"
	testService       = "test-service"
	testRoute         = "test-route"
	testConfiguration = "test-configuration"
	testRevision      = "test-revision"

	testBroker              = "test-broker"
	testEventType           = "test-eventtype"
	testEventSource         = "test-eventsource"
	testTrigger             = "test-trigger"
	testFilterType          = "test-filtertype"
	testFilterSource        = "test-filtersource"
	testSource              = "test-source"
	testSourceResourceGroup = "test-source-rg"
)

var (
	testView = &view.View{
		Description: "Test View",
		Measure:     stats.Int64("test", "Test Measure", stats.UnitNone),
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{},
	}

	nsKey = tag.Tag{Key: mustNewTagKey(metricskey.LabelNamespaceName), Value: testNS}

	serviceKey  = tag.Tag{Key: mustNewTagKey(metricskey.LabelServiceName), Value: testService}
	routeKey    = tag.Tag{Key: mustNewTagKey(metricskey.LabelRouteName), Value: testRoute}
	revisionKey = tag.Tag{Key: mustNewTagKey(metricskey.LabelRevisionName), Value: testRevision}

	brokerKey              = tag.Tag{Key: mustNewTagKey(metricskey.LabelName), Value: testBroker}
	triggerKey             = tag.Tag{Key: mustNewTagKey(metricskey.LabelName), Value: testTrigger}
	triggerBrokerKey       = tag.Tag{Key: mustNewTagKey(metricskey.LabelBrokerName), Value: testBroker}
	filterTypeKey          = tag.Tag{Key: mustNewTagKey(metricskey.LabelFilterType), Value: testFilterType}
	filterSourceKey        = tag.Tag{Key: mustNewTagKey(metricskey.LabelFilterSource), Value: testFilterSource}
	sourceKey              = tag.Tag{Key: mustNewTagKey(metricskey.LabelName), Value: testSource}
	sourceResourceGroupKey = tag.Tag{Key: mustNewTagKey(metricskey.LabelResourceGroup), Value: testSourceResourceGroup}
	eventTypeKey           = tag.Tag{Key: mustNewTagKey(metricskey.LabelEventType), Value: testEventType}
	eventSourceKey         = tag.Tag{Key: mustNewTagKey(metricskey.LabelEventSource), Value: testEventSource}

	revisionTestTags = []tag.Tag{nsKey, serviceKey, routeKey, revisionKey}

	brokerTestTags  = []tag.Tag{nsKey, brokerKey, eventTypeKey, eventSourceKey}
	triggerTestTags = []tag.Tag{nsKey, triggerKey, triggerBrokerKey, filterTypeKey, filterSourceKey}
	sourceTestTags  = []tag.Tag{nsKey, sourceKey, sourceResourceGroupKey, eventTypeKey, eventSourceKey}
)

func mustNewTagKey(s string) tag.Key {
	tagKey, err := tag.NewKey(s)
	if err != nil {
		panic(err)
	}
	return tagKey
}

func getResourceLabelValue(key string, tags []tag.Tag) string {
	for _, t := range tags {
		if t.Key.Name() == key {
			return t.Value
		}
	}
	return ""
}

func TestMain(m *testing.M) {
	resetCurPromSrv()
	// Set gcpMetadataFunc and newStackdriverExporterFunc for testing
	gcpMetadataFunc = fakeGcpMetadataFun
	newStackdriverExporterFunc = newFakeExporter
	os.Exit(m.Run())
}

func TestMetricsExporter(t *testing.T) {
	_, err := newMetricsExporter(&metricsConfig{
		domain:               servingDomain,
		component:            testComponent,
		backendDestination:   "unsupported",
		stackdriverProjectID: ""}, TestLogger(t))
	if err == nil {
		t.Errorf("Expected an error for unsupported backend %v", err)
	}

	_, err = newMetricsExporter(&metricsConfig{
		domain:               servingDomain,
		component:            testComponent,
		backendDestination:   Stackdriver,
		stackdriverProjectID: testProj}, TestLogger(t))
	if err != nil {
		t.Error(err)
	}
}

func TestInterlevedExporters(t *testing.T) {
	// Disabling this test as it fails intermittently.
	// Refer to https://github.com/knative/pkg/issues/406
	t.Skip()

	// First create a stackdriver exporter
	_, err := newMetricsExporter(&metricsConfig{
		domain:               servingDomain,
		component:            testComponent,
		backendDestination:   Stackdriver,
		stackdriverProjectID: testProj}, TestLogger(t))
	if err != nil {
		t.Error(err)
	}
	expectNoPromSrv(t)
	// Then switch to prometheus exporter
	_, err = newMetricsExporter(&metricsConfig{
		domain:             servingDomain,
		component:          testComponent,
		backendDestination: Prometheus,
		prometheusPort:     9090}, TestLogger(t))
	if err != nil {
		t.Error(err)
	}
	expectPromSrv(t, ":9090")
	// Finally switch to stackdriver exporter
	_, err = newMetricsExporter(&metricsConfig{
		domain:               servingDomain,
		component:            testComponent,
		backendDestination:   Stackdriver,
		stackdriverProjectID: testProj}, TestLogger(t))
	if err != nil {
		t.Error(err)
	}
}

func TestFlushExporter(t *testing.T) {
	// No exporter - no action should be taken
	setCurMetricsConfig(nil)
	if want, got := false, FlushExporter(); got != want {
		t.Errorf("Expected %v, got %v.", want, got)
	}

	// Prometheus exporter shouldn't do anything because
	// it doesn't implement Flush()
	c := &metricsConfig{
		domain:             servingDomain,
		component:          testComponent,
		reportingPeriod:    1 * time.Minute,
		backendDestination: Prometheus,
	}
	e, err := newMetricsExporter(c, TestLogger(t))
	if err != nil {
		t.Errorf("Expected no error. got %v", err)
	} else {
		setCurMetricsExporter(e)
		if want, got := false, FlushExporter(); got != want {
			t.Errorf("Expected %v, got %v.", want, got)
		}
	}

	// Fake Stackdriver exporter should export
	newStackdriverExporterFunc = newFakeExporter
	c = &metricsConfig{
		domain:                            servingDomain,
		component:                         testComponent,
		backendDestination:                Stackdriver,
		allowStackdriverCustomMetrics:     true,
		isStackdriverBackend:              true,
		reportingPeriod:                   1 * time.Minute,
		stackdriverProjectID:              "test",
		stackdriverMetricTypePrefix:       path.Join(servingDomain, testComponent),
		stackdriverCustomMetricTypePrefix: path.Join(customMetricTypePrefix, testComponent),
	}
	e, err = newMetricsExporter(c, TestLogger(t))
	if err != nil {
		t.Errorf("Expected no error. got %v", err)
	} else {
		setCurMetricsExporter(e)
		if want, got := true, FlushExporter(); got != want {
			t.Errorf("Expected %v, got %v.", want, got)
		}
	}

	// Real Stackdriver exporter should export as well.
	newStackdriverExporterFunc = newOpencensusSDExporter
	e, err = newMetricsExporter(c, TestLogger(t))
	if err != nil {
		t.Errorf("Expected no error. got %v", err)
	} else {
		setCurMetricsExporter(e)
		if want, got := true, FlushExporter(); got != want {
			t.Errorf("Expected %v, got %v.", want, got)
		}
	}
}
