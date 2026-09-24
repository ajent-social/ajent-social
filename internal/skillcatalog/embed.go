// SPDX-License-Identifier: Apache-2.0

package skillcatalog

import _ "embed"

// Bootstrap is the single instruction package installed by an agent.
//
//go:embed bootstrap.md
var Bootstrap string

// SeedCatalog contains reviewed public instruction-only skills and notices.
//
//go:embed seed.json
var SeedCatalog string
