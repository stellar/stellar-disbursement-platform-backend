package monitor

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseMetricType(t *testing.T) {
	testCases := []struct {
		metricTypeStr      string
		expectedMetricType MetricType
		wantErr            error
	}{
		{wantErr: fmt.Errorf("invalid metric type \"\"")},
		{metricTypeStr: "MOCKMETRICTYPE", wantErr: fmt.Errorf("invalid metric type \"MOCKMETRICTYPE\"")},
		{metricTypeStr: "prometheus", expectedMetricType: MetricTypePrometheus},
		{metricTypeStr: "PromeTHEUS", expectedMetricType: MetricTypePrometheus},
	}
	for _, tc := range testCases {
		t.Run("metricType: "+tc.metricTypeStr, func(t *testing.T) {
			metricType, err := ParseMetricType(tc.metricTypeStr)
			assert.Equal(t, tc.expectedMetricType, metricType)
			assert.Equal(t, tc.wantErr, err)
		})
	}
}

func Test_GetClient(t *testing.T) {
	metricOptions := MetricOptions{}

	t.Run("get prometheus monitor client", func(t *testing.T) {
		metricOptions.MetricType = MetricTypePrometheus

		gotClient, err := GetClient(metricOptions)
		assert.NoError(t, err)
		assert.IsType(t, &prometheusClient{}, gotClient)
	})

	t.Run("error metric passed is invalid", func(t *testing.T) {
		metricOptions.MetricType = "MOCKMETRICTYPE"

		gotClient, err := GetClient(metricOptions)
		assert.Nil(t, gotClient)
		assert.EqualError(t, err, "unknown metric type: \"MOCKMETRICTYPE\"")
	})
}
