package org_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/uptrace/terraform/internal/generated"
	"github.com/uptrace/terraform/internal/testutil"
)

func accOrgUserDataEmail(suffix string) string {
	return fmt.Sprintf("acc-org-user-ds-%d-%s@example.com", time.Now().UnixNano(), suffix)
}

func TestAccOrgUserDataSource_basic(t *testing.T) {
	email := accOrgUserDataEmail("basic")
	orgName := "acc-org-user-ds-basic"
	var orgID string

	seedConfig := fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = %q
}
`, orgName)

	readConfig := seedConfig + fmt.Sprintf(`
data "uptrace_org_user" "test" {
  org_id = uptrace_org.test.id
  email  = %q
}
`, email)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: seedConfig,
				Check:  testutil.CaptureAttr("uptrace_org.test", "id", &orgID),
			},
			{
				PreConfig: func() { seedOrgUser(t, &orgID, email, "admin") },
				Config:    readConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.uptrace_org_user.test", "id"),
					resource.TestCheckResourceAttrSet("data.uptrace_org_user.test", "user_id"),
					resource.TestCheckResourceAttr("data.uptrace_org_user.test", "email", email),
					resource.TestCheckResourceAttr("data.uptrace_org_user.test", "role", "admin"),
				),
			},
		},
	})
}

func TestAccOrgUserDataSource_notFound(t *testing.T) {
	missing := accOrgUserDataEmail("missing")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "uptrace_org" "test" {
  name = "acc-org-user-ds-missing"
}

data "uptrace_org_user" "test" {
  org_id = uptrace_org.test.id
  email  = %q
}
`, missing),
				ExpectError: regexp.MustCompile(`org user not found`),
			},
		},
	})
}

func seedOrgUser(t *testing.T, orgIDStr *string, email, role string) {
	t.Helper()
	if *orgIDStr == "" {
		t.Fatal("orgID was not captured before seeding")
	}
	orgID, err := strconv.ParseUint(*orgIDStr, 10, 64)
	if err != nil {
		t.Fatalf("invalid orgID %q: %v", *orgIDStr, err)
	}
	c := testutil.TestAccClient(t)
	userRole := generated.UserRole(role)
	inviteResp, err := c.API.CreateOrgInvite(context.Background(), &generated.CreateOrgInviteRequestOptions{
		PathParams: &generated.CreateOrgInvitePath{OrgID: orgID},
		Body: &generated.UserInviteCreateRequest{
			Email: email,
			Role:  &userRole,
		},
	})
	if err != nil {
		t.Fatalf("seed org user: create invite: %v", err)
	}
	if inviteResp.AddedToOrg != nil && *inviteResp.AddedToOrg {
		return
	}
	if _, err := c.API.CreateOrgUser(context.Background(), &generated.CreateOrgUserRequestOptions{
		PathParams: &generated.CreateOrgUserPath{OrgID: orgID},
		Body: &generated.OrgUserCreateRequest{
			UserID: inviteResp.UserID,
			Role:   userRole,
		},
	}); err != nil {
		t.Fatalf("seed org user: create org_user: %v", err)
	}
}
