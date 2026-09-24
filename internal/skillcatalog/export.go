// SPDX-License-Identifier: Apache-2.0

package skillcatalog

import (
	"encoding/json"
	"fmt"
	"io"
)

// Export writes a validated, public snapshot for the edge HTTP adapter.
// The adapter accepts no uploads; all review and validation happens here.
func (c *Catalog) Export(w io.Writer) error {
	type exportedEntry struct {
		Result        Result         `json:"result"`
		Content       string         `json:"content"`
		License       string         `json:"license"`
		LicenseSHA256 string         `json:"license_sha256"`
		Terms         map[string]int `json:"terms"`
		Length        int            `json:"length"`
	}
	entries := make([]exportedEntry, 0, len(c.entries))
	for _, e := range c.entries {
		entries = append(entries, exportedEntry{e.result, e.content, e.license, digest(e.license), e.terms, e.length})
	}
	err := json.NewEncoder(w).Encode(struct {
		Schema          string          `json:"schema"`
		Version         string          `json:"catalog_version"`
		Bootstrap       string          `json:"bootstrap"`
		BootstrapSHA256 string          `json:"bootstrap_sha256"`
		Entries         []exportedEntry `json:"entries"`
		DF              map[string]int  `json:"document_frequency"`
		AverageLength   float64         `json:"average_length"`
	}{"ajent.skills.edge/v1", c.version, c.bootstrap, digest(c.bootstrap), entries, c.documentFrequency, c.averageLength})
	if err != nil {
		return fmt.Errorf("export catalog: %w", err)
	}
	return nil
}
