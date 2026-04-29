package user_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/internal/testutil"
)

// userPreCheck reads UPTRACE_TEST_USER_EMAIL_PREFIX. Skips when unset,
// fails fast when the value lacks an `@`. Returns the prefix verbatim;
// callers add a per-process suffix to avoid colliding with prior runs.
func userPreCheck(t *testing.T) string {
	t.Helper()
	v := os.Getenv("UPTRACE_TEST_USER_EMAIL_PREFIX")
	if v == "" {
		t.Skip("UPTRACE_TEST_USER_EMAIL_PREFIX must be set to run uptrace_user acceptance tests")
	}
	if !strings.Contains(v, "@") {
		t.Fatalf("UPTRACE_TEST_USER_EMAIL_PREFIX must contain @: %q", v)
	}
	return v
}

func testAccUserConfig(email string) string {
	return fmt.Sprintf(`
resource "uptrace_user" "test" {
  email = %q
}
`, email)
}

func TestAccUser_basic(t *testing.T) {
	prefix := userPreCheck(t)
	parts := strings.SplitN(prefix, "@", 2)
	email := strings.ToLower(parts[0] + "+acc-user-" + strconv.FormatInt(int64(os.Getpid()), 10) + "@" + parts[1])

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
			{
				ResourceName:            "uptrace_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"email"},
			},
		},
	})
}
