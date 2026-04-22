package monitor_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/internal/testutil"
)

func testAccMetricMonitorConfigAuto(orgName, projectName, monitorName string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_project" "test" {
  org_id = uptrace_org.test.id
  name   = %q
}

resource "uptrace_metric_monitor" "test" {
  project_id = uptrace_project.test.id
  name       = %q

  params = {
    metrics = [
      { name = "uptrace_tracing_spans", alias = "$http_duration" }
    ]
    query = "avg($http_duration)"
    column = {
      name = "avg($http_duration)"
      unit = "milliseconds"
    }

    detector = {
      auto = {
        tolerance       = "medium"
        training_period = 86400000
      }
    }
  }
}
`, orgName, projectName, monitorName)
}

func TestAccMetricMonitor_autoDetector(t *testing.T) {
	const (
		orgName     = "acc-mm-auto-org"
		projectName = "acc-mm-auto-project"
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMonitorDestroy(t, "uptrace_metric_monitor"),
		Steps: []resource.TestStep{
			{
				Config: testAccMetricMonitorConfigAuto(orgName, projectName, "acc-mm-auto"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_metric_monitor.test", "id"),
					resource.TestCheckResourceAttr("uptrace_metric_monitor.test", "params.detector.auto.tolerance", "medium"),
					resource.TestCheckResourceAttrSet("uptrace_metric_monitor.test", "status"),
				),
			},
			{
				Config:   testAccMetricMonitorConfigAuto(orgName, projectName, "acc-mm-auto"),
				PlanOnly: true,
			},
			{
				ResourceName:      "uptrace_metric_monitor.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Ignore the whole params block: the backend normalizes MQL
				// (e.g. $http_duration → $http_duration{}), derives column defaults
				// from the query, and populates optional scalars (resolution,
				// num_eval_points, absent_points) — all of which the provider
				// intentionally preserves as null when the user did not set them.
				ImportStateVerifyIgnore: []string{"params"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["uptrace_metric_monitor.test"]
					if !ok {
						return "", fmt.Errorf("uptrace_metric_monitor.test not found in state")
					}
					return fmt.Sprintf("%s:%s", rs.Primary.Attributes["project_id"], rs.Primary.ID), nil
				},
			},
		},
	})
}
