// Package version exposes the provider version string. Override at build
// time with:
//
//	-ldflags "-X github.com/uptrace/terraform/version.ProviderVersion=v1.2.3"
package version

var ProviderVersion = "dev"
