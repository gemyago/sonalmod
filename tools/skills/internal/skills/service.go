package skills

import (
	"log/slog"
	"os"
	"path/filepath"
)

// DefaultMaxSkillBytes is the default cap on the size of a single SKILL.md file.
const DefaultMaxSkillBytes = 64 * 1024 // 64 KiB

// DefaultMaxCatalogEntries caps the total number of skills in the catalog.
const DefaultMaxCatalogEntries = 500

// CatalogEntry is a discovered skill with its metadata and full body.
type CatalogEntry struct {
	SkillEntry
}

// CatalogOption configures NewCatalog.
type CatalogOption func(*catalogOpts)

type catalogOpts struct {
	logger            *slog.Logger
	maxSkillBytes     int
	maxCatalogEntries int
}

// WithLogger sets the logger used when scanning skill roots.
// Nil falls back to [slog.Default] (same as omitting WithLogger).
func WithLogger(l *slog.Logger) CatalogOption {
	return func(o *catalogOpts) {
		o.logger = l
	}
}

// WithMaxSkillBytes sets the maximum number of bytes allowed per SKILL.md file.
func WithMaxSkillBytes(n int) CatalogOption {
	return func(o *catalogOpts) {
		if n > 0 {
			o.maxSkillBytes = n
		}
	}
}

// WithMaxCatalogEntries sets the maximum number of skills in the catalog.
func WithMaxCatalogEntries(n int) CatalogOption {
	return func(o *catalogOpts) {
		if n > 0 {
			o.maxCatalogEntries = n
		}
	}
}

// Catalog holds the in-memory index of discovered skills.
type Catalog struct {
	entries []CatalogEntry
	byName  map[string]*CatalogEntry
}

// NewCatalog builds the skill catalog by scanning the given roots.
// Non-existent roots are skipped with a warning log.
// Invalid or oversized SKILL.md files are skipped with a warning log.
// Duplicate skill names are resolved by root order (first wins).
// When no [WithLogger] option is set, logging uses [slog.Default].
func NewCatalog(roots []string, opts ...CatalogOption) (*Catalog, error) {
	o := catalogOpts{
		maxSkillBytes:     DefaultMaxSkillBytes,
		maxCatalogEntries: DefaultMaxCatalogEntries,
	}
	for _, opt := range opts {
		opt(&o)
	}
	if o.logger == nil {
		o.logger = slog.Default()
	}

	cat := &Catalog{
		byName: make(map[string]*CatalogEntry),
	}

	for _, root := range roots {
		if len(cat.entries) >= o.maxCatalogEntries {
			break
		}
		cat.scanRoot(root, o.logger, &o)
	}

	return cat, nil
}

func (c *Catalog) scanRoot(root string, logger *slog.Logger, o *catalogOpts) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		logger.Warn("skills: root directory does not exist, skipping", "path", root)
		return
	}

	dirEntries, err := os.ReadDir(root)
	if err != nil {
		logger.Warn("skills: cannot read root directory, skipping", "err", err)
		return
	}

	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		if len(c.entries) >= o.maxCatalogEntries {
			logger.Warn("skills: catalog entry limit reached, skipping remaining skills",
				"limit", o.maxCatalogEntries)
			break
		}
		c.loadSkillDir(filepath.Join(root, de.Name()), de.Name(), logger, o)
	}
}

func (c *Catalog) loadSkillDir(skillDir, dirName string, logger *slog.Logger, o *catalogOpts) {
	skillFile := filepath.Join(skillDir, "SKILL.md")

	info, statErr := os.Stat(skillFile)
	if os.IsNotExist(statErr) {
		return
	}
	if statErr != nil {
		logger.Warn("skills: cannot stat SKILL.md, skipping", "dir", dirName, "err", statErr)
		return
	}
	if int(info.Size()) > o.maxSkillBytes {
		logger.Warn("skills: SKILL.md exceeds size limit, skipping",
			"dir", dirName, "size", info.Size(), "limit", o.maxSkillBytes)
		return
	}

	content, err := os.ReadFile(skillFile)
	if err != nil {
		logger.Warn("skills: cannot read SKILL.md, skipping", "dir", dirName, "err", err)
		return
	}

	entry, err := parseSkillFile(content, dirName)
	if err != nil {
		logger.Warn("skills: invalid SKILL.md, skipping", "dir", dirName, "err", err)
		return
	}

	if _, exists := c.byName[entry.Name]; exists {
		logger.Warn("skills: duplicate skill name, keeping first occurrence", "name", entry.Name)
		return
	}

	ce := CatalogEntry{SkillEntry: entry}
	c.entries = append(c.entries, ce)
	c.byName[entry.Name] = &c.entries[len(c.entries)-1]
}

// List returns metadata for all discovered skills in catalog order.
func (c *Catalog) List() []CatalogEntry {
	if c == nil {
		return nil
	}
	return c.entries
}

// Get returns the CatalogEntry for the named skill, or false if not found.
func (c *Catalog) Get(name string) (CatalogEntry, bool) {
	if c == nil {
		return CatalogEntry{}, false
	}
	e, ok := c.byName[name]
	if !ok {
		return CatalogEntry{}, false
	}
	return *e, true
}
