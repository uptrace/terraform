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

// testAccCheckOrgUserDestroy verifies that every uptrace_org_user in the
// post-destroy state is gone from the backend. Under the invite-then-accept
// model, a resource may end its life in one of two shapes:
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

			if ouID := rs.Primary.Attributes["org_user_id"]; ouID != "" {
				orgUserID, err := strconv.ParseUint(ouID, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid org_user_id %q: %w", ouID, err)
				}
				_, err = c.API.GetOrgUser(context.Background(), &generated.GetOrgUserRequestOptions{
					PathParams: &generated.GetOrgUserPath{OrgID: orgID, OrgUserID: orgUserID},
				})
				if err == nil {
					return fmt.Errorf("org_user %d still exists after destroy", orgUserID)
				}
				if !client.IsNotFound(err) && !client.IsForbidden(err) {
					return fmt.Errorf("checking org_user %d after destroy: %w", orgUserID, err)
				}
			}

			inviteID := rs.Primary.ID
			out, err := c.API.ListOrgInvites(context.Background(), &generated.ListOrgInvitesRequestOptions{
				PathParams: &generated.ListOrgInvitesPath{OrgID: orgID},
			})
			if err != nil {
				if client.IsNotFound(err) || client.IsForbidden(err) {
					continue
				}
				return fmt.Errorf("listing invites after destroy: %w", err)
			}
			for i := range out.Invites {
				if out.Invites[i].ID == inviteID && out.Invites[i].State == generated.Sent {
					return fmt.Errorf("invite %q still in sent state after destroy", inviteID)
				}
			}
		}
		return nil
	}
}

// TestAccOrgUser_basic exercises the full invite → accept → role-update →
// import → destroy lifecycle. Step 3 simulates the invitee clicking the
// emailed /join link by calling JoinOrg out-of-band; the subsequent refresh
// is what transitions the resource from `sent` to `accepted` and populates
// `org_user_id` / `user_id`.
func TestAccOrgUser_basic(t *testing.T) {
	email := accOrgUserEmail("basic")
	config := testAccOrgUserConfig("acc-org-user-org", email, "member")
	configAdmin := testAccOrgUserConfig("acc-org-user-org", email, "admin")
	var inviteID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptrace_org_user.test", "id"),
					resource.TestCheckResourceAttr("uptrace_org_user.test", "state", "sent"),
					resource.TestCheckNoResourceAttr("uptrace_org_user.test", "org_user_id"),
					resource.TestCheckNoResourceAttr("uptrace_org_user.test", "user_id"),
					resource.TestCheckResourceAttr("uptrace_org_user.test", "email", email),
					resource.TestCheckResourceAttr("uptrace_org_user.test", "role", "member"),
					testutil.CaptureAttr("uptrace_org_user.test", "id", &inviteID),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
			{
				PreConfig: func() { acceptInviteOutOfBand(t, &inviteID) },
				Config:    config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_user.test", "state", "accepted"),
					resource.TestCheckResourceAttrSet("uptrace_org_user.test", "org_user_id"),
					resource.TestCheckResourceAttrSet("uptrace_org_user.test", "user_id"),
					resource.TestCheckResourceAttr("uptrace_org_user.test", "role", "member"),
				),
			},
			{
				Config: configAdmin,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_user.test", "role", "admin"),
				),
			},
			{
				ResourceName:      "uptrace_org_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: orgUserImportStateIDFunc("uptrace_org_user.test"),
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

// TestAccOrgUser_pendingDestroyCancelsInvite verifies that destroying a
// resource that never reached `accepted` takes the CancelOrgInvite path.
// The framework's end-of-test destroy + CheckDestroy covers the assertion.
func TestAccOrgUser_pendingDestroyCancelsInvite(t *testing.T) {
	email := accOrgUserEmail("pending")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccOrgUserConfig("acc-org-user-pending-org", email, "member"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_user.test", "state", "sent"),
					resource.TestCheckNoResourceAttr("uptrace_org_user.test", "org_user_id"),
				),
			},
		},
	})
}

// TestAccOrgUser_disappearsOutOfBand covers the accepted-member path: the
// OrgUser is removed directly via the API while Terraform still holds state
// for it, and the next apply is expected to re-invite and reconcile.
func TestAccOrgUser_disappearsOutOfBand(t *testing.T) {
	email := accOrgUserEmail("disappear")
	config := testAccOrgUserConfig("acc-org-user-disappear-org", email, "member")
	var orgID, inviteID, orgUserID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testutil.PreCheck(t) },
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckOrgUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testutil.CaptureAttr("uptrace_org_user.test", "org_id", &orgID),
					testutil.CaptureAttr("uptrace_org_user.test", "id", &inviteID),
				),
			},
			{
				PreConfig: func() { acceptInviteOutOfBand(t, &inviteID) },
				Config:    config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptrace_org_user.test", "state", "accepted"),
					testutil.CaptureAttr("uptrace_org_user.test", "org_user_id", &orgUserID),
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

// acceptInviteOutOfBand simulates the invitee clicking the emailed /join
// link. The /join endpoint is public (no auth) and takes only the invite id.
func acceptInviteOutOfBand(t *testing.T, inviteID *string) {
	t.Helper()
	if inviteID == nil || *inviteID == "" {
		t.Fatal("invite id was not captured before accept")
	}
	c := testutil.TestAccClient(t)
	_, err := c.API.JoinOrg(context.Background(), &generated.JoinOrgRequestOptions{
		PathParams: &generated.JoinOrgPath{InviteID: *inviteID},
	})
	if err != nil {
		t.Fatalf("accept invite %q out-of-band: %v", *inviteID, err)
	}
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
