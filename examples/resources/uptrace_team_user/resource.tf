resource "uptrace_org" "example" {
  name = "My Organization"
}

resource "uptrace_team" "example" {
  org_id = uptrace_org.example.id
  name   = "platform"
}

data "uptrace_org_user" "admin" {
  org_id = uptrace_org.example.id
  email  = "admin@uptrace.local"
}

resource "uptrace_team_user" "example" {
  org_id      = uptrace_org.example.id
  team_id     = uptrace_team.example.id
  org_user_id = data.uptrace_org_user.admin.id
}
