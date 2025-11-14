package resilience_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	resilience "github.com/JohnPlummer/jp-go-resilience"
)

var _ = Describe("HealthStatus", func() {
	Describe("JSON Marshaling", func() {
		It("should marshal to JSON correctly", func() {
			health := resilience.HealthStatus{
				Healthy:              true,
				Status:               "closed",
				State:                "closed",
				Requests:             10,
				TotalSuccesses:       8,
				TotalFailures:        2,
				ConsecutiveFailures:  0,
				ConsecutiveSuccesses: 2,
			}

			data, err := json.Marshal(health)
			Expect(err).To(BeNil())

			var unmarshaled map[string]interface{}
			err = json.Unmarshal(data, &unmarshaled)
			Expect(err).To(BeNil())

			Expect(unmarshaled["healthy"]).To(BeTrue())
			Expect(unmarshaled["status"]).To(Equal("closed"))
			Expect(unmarshaled["state"]).To(Equal("closed"))
			Expect(unmarshaled["requests"]).To(BeNumerically("==", 10))
			Expect(unmarshaled["total_successes"]).To(BeNumerically("==", 8))
			Expect(unmarshaled["total_failures"]).To(BeNumerically("==", 2))
			Expect(unmarshaled["consecutive_failures"]).To(BeNumerically("==", 0))
			Expect(unmarshaled["consecutive_successes"]).To(BeNumerically("==", 2))
		})

		It("should unmarshal from JSON correctly", func() {
			jsonData := `{
				"healthy": false,
				"status": "open",
				"state": "open",
				"requests": 5,
				"total_successes": 0,
				"total_failures": 5,
				"consecutive_failures": 5,
				"consecutive_successes": 0
			}`

			var health resilience.HealthStatus
			err := json.Unmarshal([]byte(jsonData), &health)
			Expect(err).To(BeNil())

			Expect(health.Healthy).To(BeFalse())
			Expect(health.Status).To(Equal("open"))
			Expect(health.State).To(Equal("open"))
			Expect(health.Requests).To(Equal(uint32(5)))
			Expect(health.TotalSuccesses).To(Equal(uint32(0)))
			Expect(health.TotalFailures).To(Equal(uint32(5)))
			Expect(health.ConsecutiveFailures).To(Equal(uint32(5)))
			Expect(health.ConsecutiveSuccesses).To(Equal(uint32(0)))
		})
	})

	Describe("Field Values", func() {
		It("should store all fields correctly", func() {
			health := resilience.HealthStatus{
				Healthy:              true,
				Status:               "half-open",
				State:                "half-open",
				Requests:             3,
				TotalSuccesses:       2,
				TotalFailures:        1,
				ConsecutiveFailures:  0,
				ConsecutiveSuccesses: 1,
			}

			Expect(health.Healthy).To(BeTrue())
			Expect(health.Status).To(Equal("half-open"))
			Expect(health.State).To(Equal("half-open"))
			Expect(health.Requests).To(Equal(uint32(3)))
			Expect(health.TotalSuccesses).To(Equal(uint32(2)))
			Expect(health.TotalFailures).To(Equal(uint32(1)))
			Expect(health.ConsecutiveFailures).To(Equal(uint32(0)))
			Expect(health.ConsecutiveSuccesses).To(Equal(uint32(1)))
		})
	})

	Describe("Zero Values", func() {
		It("should have sensible zero values", func() {
			var health resilience.HealthStatus

			Expect(health.Healthy).To(BeFalse())
			Expect(health.Status).To(BeEmpty())
			Expect(health.State).To(BeEmpty())
			Expect(health.Requests).To(Equal(uint32(0)))
			Expect(health.TotalSuccesses).To(Equal(uint32(0)))
			Expect(health.TotalFailures).To(Equal(uint32(0)))
			Expect(health.ConsecutiveFailures).To(Equal(uint32(0)))
			Expect(health.ConsecutiveSuccesses).To(Equal(uint32(0)))
		})
	})
})
