resource "uptrace_org" "main" {
  name = "Main"
}

resource "uptrace_user" "alice" {
  email = "alice@example.com"
}

resource "uptrace_org_user" "alice" {
  org_id  = uptrace_org.main.id
  user_id = uptrace_user.alice.id
  role    = "admin"
}

resource "uptrace_team" "platform" {
  org_id = uptrace_org.main.id
  name   = "Platform"
}

resource "uptrace_team_user" "alice_platform" {
  org_id      = uptrace_org.main.id
  team_id     = uptrace_team.platform.id
  org_user_id = uptrace_org_user.alice.id
}
