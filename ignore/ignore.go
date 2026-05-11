package ignore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
)

var (
	ErrInvalidBasePath       = errors.New("invalid ignore base path")
	ErrInvalidIgnorePath     = errors.New("invalid ignore path")
	ErrMissingFilesystem     = errors.New("missing filesystem")
	ErrIgnorePathIsDirectory = errors.New("ignore path is a directory")
)

// Ignores is a preprocessed list of gitignore-style rules rooted at one iRODS
// absolute base path.
type Ignores struct {
	basePath string
	entries  []IgnoreEntry
}

// IgnoreEntry is one preprocessed ignore rule.
type IgnoreEntry struct {
	Raw           string
	Pattern       string
	Negate        bool
	DirectoryOnly bool
	Anchored      bool
	hasSlash      bool
	segments      []string
}

// IRODSFileHandleReader is the read/close subset required to load an ignore
// file from iRODS.
type IRODSFileHandleReader interface {
	ReadAt(buffer []byte, offset int64) (int, error)
	Close() error
}

// IRODSIgnoreFilesystem is the minimal iRODS API required to read one ignore
// file.
type IRODSIgnoreFilesystem interface {
	Stat(irodsPath string) (*irodsfs.Entry, error)
	OpenFile(irodsPath string, resource string, mode string) (IRODSFileHandleReader, error)
}

// NewIgnores preprocesses ignore-pattern lines into a reusable matcher.
// basePath must be an iRODS absolute path where the ignore file is rooted.
func NewIgnores(basePath string, lines []string) (*Ignores, error) {
	normalizedBasePath, ok := normalizeAbsolutePath(basePath)
	if !ok {
		return nil, ErrInvalidBasePath
	}

	entries := make([]IgnoreEntry, 0, len(lines))
	for _, line := range lines {
		entry, ok := preprocessLine(line)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}

	return &Ignores{
		basePath: normalizedBasePath,
		entries:  entries,
	}, nil
}

// ParseIgnoreFileContents parses raw ignore-file contents into Ignores.
func ParseIgnoreFileContents(basePath string, contents string) (*Ignores, error) {
	contents = strings.ReplaceAll(contents, "\r\n", "\n")
	contents = strings.ReplaceAll(contents, "\r", "\n")
	return NewIgnores(basePath, strings.Split(contents, "\n"))
}

// ReadIgnoreFile reads and preprocesses a local ignore file.
func ReadIgnoreFile(ignoreFilePath string, basePath string) (*Ignores, error) {
	return ReadIgnoreFileFromLocal(ignoreFilePath, basePath)
}

// ReadIgnoreFileFromLocal reads and preprocesses a local ignore file.
func ReadIgnoreFileFromLocal(ignoreFilePath string, basePath string) (*Ignores, error) {
	bytesValue, err := os.ReadFile(strings.TrimSpace(ignoreFilePath))
	if err != nil {
		return nil, err
	}
	return ParseIgnoreFileContents(basePath, string(bytesValue))
}

// ReadIgnoreFileFromIRODS reads and preprocesses an ignore file stored in iRODS.
func ReadIgnoreFileFromIRODS(filesystem IRODSIgnoreFilesystem, ignoreIRODSPath string, basePath string) (*Ignores, error) {
	if filesystem == nil {
		return nil, ErrMissingFilesystem
	}

	ignoreIRODSPath = strings.TrimSpace(ignoreIRODSPath)
	if ignoreIRODSPath == "" {
		return nil, fmt.Errorf("%w: %w", ErrInvalidIgnorePath, os.ErrInvalid)
	}

	entry, err := filesystem.Stat(ignoreIRODSPath)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, os.ErrNotExist
	}
	if entry.IsDir() {
		return nil, ErrIgnorePathIsDirectory
	}
	if entry.Size == 0 {
		return NewIgnores(basePath, []string{})
	}

	handle, err := filesystem.OpenFile(ignoreIRODSPath, "", "r")
	if err != nil {
		return nil, err
	}
	defer handle.Close() //nolint:errcheck

	buffer := make([]byte, entry.Size)
	read, readErr := handle.ReadAt(buffer, 0)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, readErr
	}

	return ParseIgnoreFileContents(basePath, string(buffer[:read]))
}

// BasePath returns the iRODS absolute base path used to interpret rules.
func (ignores *Ignores) BasePath() string {
	if ignores == nil {
		return ""
	}
	return ignores.basePath
}

// Entries returns a copy of the preprocessed ignore entries.
func (ignores *Ignores) Entries() []IgnoreEntry {
	if ignores == nil || len(ignores.entries) == 0 {
		return []IgnoreEntry{}
	}

	copied := make([]IgnoreEntry, 0, len(ignores.entries))
	for _, entry := range ignores.entries {
		entryCopy := entry
		entryCopy.segments = append([]string(nil), entry.segments...)
		copied = append(copied, entryCopy)
	}
	return copied
}

// FilterEntries filters list output from iRODS FileSystem.List-style calls.
// Entries with nil pointers are skipped.
func FilterEntries(ignores *Ignores, entries []*irodsfs.Entry) []*irodsfs.Entry {
	if len(entries) == 0 {
		return []*irodsfs.Entry{}
	}
	if ignores == nil || len(ignores.entries) == 0 {
		filtered := make([]*irodsfs.Entry, 0, len(entries))
		for _, entry := range entries {
			if entry != nil {
				filtered = append(filtered, entry)
			}
		}
		return filtered
	}

	filtered := make([]*irodsfs.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if !ignores.IsIgnored(entry.Path, entry.IsDir()) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// IsIgnored evaluates one iRODS absolute path against preprocessed rules.
func (ignores *Ignores) IsIgnored(irodsPath string, isDir bool) bool {
	if ignores == nil || len(ignores.entries) == 0 {
		return false
	}

	relative, inside := relativeToBase(ignores.basePath, irodsPath)
	if !inside {
		return false
	}

	segments := splitPathComponents(relative)
	ignored := false

	for index, entry := range ignores.entries {
		if !entry.matches(segments, isDir) {
			continue
		}

		if entry.Negate {
			// Match gitignore behavior: a negated pattern cannot re-include a
			// path when any parent directory remains ignored.
			if ignores.parentDirectoryIgnoredBefore(segments, index) {
				continue
			}
			ignored = false
			continue
		}

		ignored = true
	}

	return ignored
}

func (ignores *Ignores) parentDirectoryIgnoredBefore(segments []string, maxExclusive int) bool {
	if len(segments) <= 1 || maxExclusive <= 0 {
		return false
	}

	// For files, every component except the basename is a parent directory.
	// For directories, every component except the full path is a parent.
	for parentLength := 1; parentLength < len(segments); parentLength++ {
		if ignores.isIgnoredPrefix(segments[:parentLength], true, maxExclusive) {
			return true
		}
	}
	return false
}

func (ignores *Ignores) isIgnoredPrefix(segments []string, isDir bool, maxExclusive int) bool {
	ignored := false
	for index := 0; index < maxExclusive && index < len(ignores.entries); index++ {
		entry := ignores.entries[index]
		if !entry.matches(segments, isDir) {
			continue
		}
		if entry.Negate {
			ignored = false
		} else {
			ignored = true
		}
	}
	return ignored
}

func preprocessLine(line string) (IgnoreEntry, bool) {
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	line = trimTrailingUnescapedSpaces(line)
	if line == "" {
		return IgnoreEntry{}, false
	}

	// Handle escaped comment and negation markers at the first character.
	if strings.HasPrefix(line, `\#`) {
		line = line[1:]
	} else if strings.HasPrefix(line, "#") {
		return IgnoreEntry{}, false
	}

	negate := false
	if strings.HasPrefix(line, `\!`) {
		line = line[1:]
	} else if strings.HasPrefix(line, "!") {
		negate = true
		line = line[1:]
	}

	if line == "" {
		return IgnoreEntry{}, false
	}

	directoryOnly := strings.HasSuffix(line, "/")
	if directoryOnly {
		line = strings.TrimSuffix(line, "/")
	}
	if line == "" {
		return IgnoreEntry{}, false
	}

	hadLeadingSlash := strings.HasPrefix(line, "/")
	line = strings.TrimPrefix(line, "/")

	hasSlash := strings.Contains(line, "/")
	anchored := hadLeadingSlash || hasSlash

	segments := splitPathComponents(line)
	if len(segments) == 0 {
		return IgnoreEntry{}, false
	}

	return IgnoreEntry{
		Raw:           line,
		Pattern:       line,
		Negate:        negate,
		DirectoryOnly: directoryOnly,
		Anchored:      anchored,
		hasSlash:      hasSlash,
		segments:      segments,
	}, true
}

func (entry IgnoreEntry) matches(pathSegments []string, isDir bool) bool {
	if len(pathSegments) == 0 {
		return false
	}

	if entry.DirectoryOnly {
		maxPrefix := len(pathSegments)
		if !isDir {
			maxPrefix = len(pathSegments) - 1
		}
		for prefixLength := 1; prefixLength <= maxPrefix; prefixLength++ {
			if entry.matchesPath(pathSegments[:prefixLength]) {
				return true
			}
		}
		return false
	}

	return entry.matchesPath(pathSegments)
}

func (entry IgnoreEntry) matchesPath(pathSegments []string) bool {
	if len(pathSegments) == 0 {
		return false
	}

	if !entry.hasSlash && !entry.Anchored {
		pattern := entry.Pattern
		for _, segment := range pathSegments {
			if matchPathSegment(pattern, segment) {
				return true
			}
		}
		return false
	}

	return matchPatternSegments(entry.segments, pathSegments)
}

func matchPatternSegments(patternSegments []string, pathSegments []string) bool {
	if len(patternSegments) == 0 {
		return len(pathSegments) == 0
	}

	// Collapse duplicate ** segments to avoid redundant recursion.
	if len(patternSegments) > 1 && patternSegments[0] == "**" && patternSegments[1] == "**" {
		return matchPatternSegments(patternSegments[1:], pathSegments)
	}

	if patternSegments[0] == "**" {
		if len(patternSegments) == 1 {
			return true
		}

		for idx := 0; idx <= len(pathSegments); idx++ {
			if matchPatternSegments(patternSegments[1:], pathSegments[idx:]) {
				return true
			}
		}
		return false
	}

	if len(pathSegments) == 0 {
		return false
	}

	if !matchPathSegment(patternSegments[0], pathSegments[0]) {
		return false
	}

	return matchPatternSegments(patternSegments[1:], pathSegments[1:])
}

func matchPathSegment(pattern string, value string) bool {
	ok, err := path.Match(pattern, value)
	if err != nil {
		// Invalid patterns are treated as non-matching.
		return false
	}
	return ok
}

func trimTrailingUnescapedSpaces(value string) string {
	end := len(value)
	for end > 0 && value[end-1] == ' ' {
		backslashes := 0
		for idx := end - 2; idx >= 0 && value[idx] == '\\'; idx-- {
			backslashes++
		}
		if backslashes%2 == 1 {
			// Escaped trailing space should remain.
			break
		}
		end--
	}
	return value[:end]
}

func relativeToBase(basePath string, absolutePath string) (string, bool) {
	normalizedPath, ok := normalizeAbsolutePath(absolutePath)
	if !ok {
		return "", false
	}

	if basePath == "/" {
		return strings.TrimPrefix(normalizedPath, "/"), true
	}

	if normalizedPath == basePath {
		return path.Base(basePath), true
	}

	prefix := basePath + "/"
	if strings.HasPrefix(normalizedPath, prefix) {
		return strings.TrimPrefix(normalizedPath, prefix), true
	}

	return "", false
}

func normalizeAbsolutePath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") {
		return "", false
	}
	normalized := path.Clean(value)
	if normalized == "." || normalized == "" {
		return "", false
	}
	if !strings.HasPrefix(normalized, "/") {
		return "", false
	}
	return normalized, true
}

func splitPathComponents(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "/")
	if value == "" {
		return []string{}
	}

	parts := strings.Split(value, "/")
	components := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		components = append(components, part)
	}
	return components
}

func (entry IgnoreEntry) String() string {
	prefix := ""
	if entry.Negate {
		prefix = "!"
	}
	suffix := ""
	if entry.DirectoryOnly {
		suffix = "/"
	}
	return fmt.Sprintf("%s%s%s", prefix, entry.Pattern, suffix)
}
