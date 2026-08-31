package doccheck_test

import (
	"bufio"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var (
	markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
	headingPattern      = regexp.MustCompile(`^ {0,3}#{1,6}[\t ]+(.+?)[\t ]*#*[\t ]*$`)
	htmlTagPattern      = regexp.MustCompile(`<[^>]+>`)
)

func TestLocalMarkdownLinks(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))

	var markdownFiles []string
	err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			markdownFiles = append(markdownFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	anchorCache := make(map[string]map[string]struct{})
	for _, markdownFile := range markdownFiles {
		checkMarkdownFile(t, repositoryRoot, markdownFile, anchorCache)
	}
}

func checkMarkdownFile(t *testing.T, repositoryRoot, markdownFile string, anchorCache map[string]map[string]struct{}) {
	t.Helper()

	file, err := os.Open(markdownFile)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	inFence := false
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		line = removeInlineCode(line)
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(line, -1) {
			checkLink(t, repositoryRoot, markdownFile, lineNumber, parseDestination(match[1]), anchorCache)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Errorf("%s: %v", markdownFile, err)
	}
}

func checkLink(t *testing.T, repositoryRoot, sourceFile string, lineNumber int, destination string, anchorCache map[string]map[string]struct{}) {
	t.Helper()
	if destination == "" {
		return
	}

	parsed, err := url.Parse(destination)
	if err != nil {
		t.Errorf("%s:%d: parse link %q: %v", sourceFile, lineNumber, destination, err)
		return
	}
	if parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(destination, "//") {
		return
	}

	targetFile := sourceFile
	if parsed.Path != "" {
		decodedPath, decodeErr := url.PathUnescape(parsed.Path)
		if decodeErr != nil {
			t.Errorf("%s:%d: decode link %q: %v", sourceFile, lineNumber, destination, decodeErr)
			return
		}
		targetFile = filepath.Clean(filepath.Join(filepath.Dir(sourceFile), filepath.FromSlash(decodedPath)))
	}

	relativeTarget, err := filepath.Rel(repositoryRoot, targetFile)
	if err != nil || relativeTarget == ".." || strings.HasPrefix(relativeTarget, ".."+string(filepath.Separator)) {
		t.Errorf("%s:%d: local link %q escapes the repository", sourceFile, lineNumber, destination)
		return
	}
	info, err := os.Stat(targetFile)
	if err != nil {
		t.Errorf("%s:%d: local link %q points to a missing target", sourceFile, lineNumber, destination)
		return
	}
	if parsed.Fragment == "" || info.IsDir() || !strings.EqualFold(filepath.Ext(targetFile), ".md") {
		return
	}

	fragment, err := url.PathUnescape(parsed.Fragment)
	if err != nil {
		t.Errorf("%s:%d: decode anchor in %q: %v", sourceFile, lineNumber, destination, err)
		return
	}
	anchors, ok := anchorCache[targetFile]
	if !ok {
		anchors = markdownAnchors(t, targetFile)
		anchorCache[targetFile] = anchors
	}
	if _, ok := anchors[strings.ToLower(fragment)]; !ok {
		t.Errorf("%s:%d: local link %q points to a missing heading", sourceFile, lineNumber, destination)
	}
}

func markdownAnchors(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Error(err)
		return nil
	}
	defer func() { _ = file.Close() }()

	anchors := make(map[string]struct{})
	duplicates := make(map[string]int)
	scanner := bufio.NewScanner(file)
	inFence := false
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		match := headingPattern.FindStringSubmatch(scanner.Text())
		if len(match) == 0 {
			continue
		}
		base := githubHeadingSlug(match[1])
		anchor := base
		if duplicate := duplicates[base]; duplicate > 0 {
			anchor += "-" + strconv.Itoa(duplicate)
		}
		duplicates[base]++
		anchors[anchor] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		t.Errorf("%s: %v", path, err)
	}
	return anchors
}

func githubHeadingSlug(heading string) string {
	heading = html.UnescapeString(htmlTagPattern.ReplaceAllString(heading, ""))
	heading = strings.ToLower(heading)
	var slug strings.Builder
	for _, char := range heading {
		switch {
		case unicode.IsLetter(char), unicode.IsNumber(char), char == '-', char == '_':
			slug.WriteRune(char)
		case unicode.IsSpace(char):
			slug.WriteByte('-')
		}
	}
	return slug.String()
}

func removeInlineCode(line string) string {
	var result strings.Builder
	inCode := false
	for _, char := range line {
		if char == '`' {
			inCode = !inCode
			continue
		}
		if !inCode {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func parseDestination(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "<") {
		if end := strings.IndexByte(raw, '>'); end >= 0 {
			return raw[1:end]
		}
	}
	if fields := strings.Fields(raw); len(fields) > 0 {
		return fields[0]
	}
	return ""
}
