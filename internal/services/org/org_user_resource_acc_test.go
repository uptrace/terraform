package org_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/uptrace/terraform/internal/client"
	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/testutil"
)

// accOrgUserEmail returns a unique @example.com address so test runs don't
// collide and no real inbox ever receives mail. example.com is reserved by
// RFC 2606 and guaranteed not to accept delivery.
func accOrgUserEmail(suffix string) string {
	return fmt.Sprintf("acc-org-user-%d-%s@example.com", time.Now().UnixNano(), suffix)
}

func testAccOrgUserConfig(orgName, email, role string) string {
	return fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}

resource "uptrace_org_user" "test" {
  org_id = uptrace_org.test.id
  email  = %q
  role   = %q
}
`, orgName, email, role)
}

func testAccCheckOrgUserDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testutil.TestAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "uptrace_org_user" {
				continue
			}
			orgID, err := strconv.ParseUint(rs.Primary.Attributes["org_id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid org_id %q: %w", rs.Primary.Attributes["org_id"], err)
			}
			orgUserID, err := strconv.ParseUint(rs.Primary.ID, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid org_user_id %q: %w", rs.Primary.ID, err)
			}
			_, err = c.API.GetOrgUser(context.Background(), &generated.GetOrgUserRequestOptions{
				PathParams: &generated.GetOrgUserPath{OrgID: orgID, OrgUserID: orgUserID},
			})
			if err == nil {
				return fmt.Errorf("org_user %d still exists after destroy", orgUserID)
			}
			// Accept NotFound (user removed) and Forbidden (parent org
			// gone, access denied) as successful destroys.
			if !client.IsNotFound(err) && !client.IsForbidden(err) {
				return fmt.Errorf("checking org_user %d after destroy: %w", orgUserID, err)
			}
		}
		return nil
	}
}

func TestAccOrgUser_basic(t *testing.T) {
	email := accOrgUserEmail("basic")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgUserConfig("acc-org-user-org", email, "member"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_org_user.test", "id"),
					resource.TestCheckResourceAttrSet("uptrace_org_user.test", "org_id"),
					resource.TestCheckResourceAttrSet("uptrace_org_user.test", "user_id"),
					resource.TestCheckResourceAttrSet("uptrace_org_user.test", "invite_id"),
					resource.TestCheckResourceAttr("uptrace_org_user.test", "email", email),
					resource.TestCheckResourceAttr("uptrace_org_user.test", "role", "member"),
				),
			},
			{
				Config:   testAccOrgUserConfig("acc-org-user-org", email, "member"),
				PlanOnly: true,
			},
			{
				Config: testAccOrgUserConfig("acc-org-user-org", email, "admin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_user.test", "role", "admin"),
				),
			},
			{
				ResourceName:      "uptrace_org_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				// invite_id is Create-time only — not recoverable on import.
				ImportStateVerifyIgnore: []string{"invite_id"},
				ImportStateIdFunc:       orgUserImportStateIDFunc("uptrace_org_user.test"),
			},
		},
	})
}

func TestAccOrgUser_mixedCaseEmailRejected(t *testing.T) {
	mixed := fmt.Sprintf("Acc-Org-User-Mixed-%d@Example.com", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      testAccOrgUserConfig("acc-org-user-mixed-org", mixed, "member"),
				ExpectError: regexp.MustCompile(`email must be lowercase and trimmed`),
			},
		},
	})
}

func TestAccOrgUser_disappearsOutOfBand(t *testing.T) {
	email := accOrgUserEmail("disappear")
	config := testAccOrgUserConfig("acc-org-user-disappear-org", email, "member")
	var orgID, orgUserID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testutil.CaptureAttr("uptrace_org_user.test", "org_id", &orgID),
					testutil.CaptureAttr("uptrace_org_user.test", "id", &orgUserID),
				),
			},
			{
				PreConfig: func() { removeOrgUserOutOfBand(t, orgID, orgUserID) },
				Config:    config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_user.test", "email", email),
					resource.TestCheckResourceAttr("uptrace_org_user.test", "role", "member"),
				),
			},
		},
	})
}

func removeOrgUserOutOfBand(t *testing.T, orgIDStr, orgUserIDStr string) {
	t.Helper()
	if orgIDStr == "" || orgUserIDStr == "" {
		t.Fatal("org_id or org_user_id was not captured before out-of-band delete")
	}
	orgID, err := strconv.ParseUint(orgIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid org_id %q: %v", orgIDStr, err)
	}
	orgUserID, err := strconv.ParseUint(orgUserIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid org_user_id %q: %v", orgUserIDStr, err)
	}
	c := testutil.TestAccClient(t)
	_, err = c.API.RemoveOrgUser(context.Background(), &generated.RemoveOrgUserRequestOptions{
		PathParams: &generated.RemoveOrgUserPath{OrgID: orgID, OrgUserID: orgUserID},
	})
	if err != nil && !client.IsNotFound(err) && !client.IsForbidden(err) {
		t.Fatalf("remove org_user %d out-of-band: %v", orgUserID, err)
	}
}

func orgUserImportStateIDFunc(resourceAddr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return "", fmt.Errorf("resource %s not found", resourceAddr)
		}
		return fmt.Sprintf("%s:%s", rs.Primary.Attributes["org_id"], rs.Primary.ID), nil
	}
}
