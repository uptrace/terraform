resource "uptrace_org" "example" {
  name = "My Organization"
}

resource "uptrace_team" "example" {
  org_id = uptrace_org.example.id
  name   = "platform"
}

resource "uptrace_user" "example" {
  email = "alice@example.com"
}

resource "uptrace_org_user" "example" {
  org_id  = uptrace_org.example.id
  user_id = uptrace_user.example.id
  role    = "admin"
}

resource "uptrace_team_user" "example" {
  org_id      = uptrace_org.example.id
  team_id     = uptrace_team.example.id
  org_user_id = uptrace_org_user.example.id
}
