resource "uptrace_org" "example" {
  name = "My Organization"
}

resource "uptrace_org_user" "example" {
  org_id = uptrace_org.example.id
  email  = "alice@example.com"
  role   = "member"
}
