// Package presets holds the presets shipped inside the burrow binary.
//
// Each *.preset file here is built in: `burrow run NAME` and `burrow exec NAME` find it when the
// user has no preset of that name in their own presets folder. Every image is pinned by digest,
// and Dependabot cannot see these files, so a pin is updated by hand (docs/DEVELOPMENT.md).
package presets

import "embed"

// FS contains the built-in presets.
//
//go:embed *.preset
var FS embed.FS
