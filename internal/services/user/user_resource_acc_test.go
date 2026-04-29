package user_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/internal/testutil"
)

func testAccUserConfig(email string) string {
	return fmt.Sprintf(`
resource "uptrace_user" "test" {
  email = %q
}
`, email)
}

func TestAccUser_basic(t *testing.T) {
	email := testutil.AcceptanceTestUserEmail("acc-user")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig(email),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_user.test", "email", email),
					resource.TestCheckResourceAttrSet("uptrace_user.test", "id"),
				),
			},
			{
				Config:   testAccUserConfig(email),
				PlanOnly: true,
			},
		},
	})
}
