package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var markdownLink = regexp.MustCompile("!?\\[[^\\]]*\\]\\(([^)]+)\\)")

type problem struct {
	file   string
	line   int
	target string
	reason string
}

func main() {
	remote := flag.Bool("remote", false, "also check HTTP and HTTPS links")
	flag.Parse()
	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		fmt.Fprintln(os.Stderr, "Please provide an existing directory.")
		os.Exit(2)
	}
	client := &http.Client{Timeout: 8 * time.Second}
	problems := make([]problem, 0)
	files := 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			fmt.Fprintln(os.Stderr, "Skipping unreadable path:", path)
			return nil
		}
		if entry.IsDir() && path != root {
			switch entry.Name() {
			case ".git", "node_modules", "vendor", "dist":
				return filepath.SkipDir
			}
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		files++
		found, scanErr := scanFile(root, path, *remote, client)
		if scanErr != nil {
			return scanErr
		}
		problems = append(problems, found...)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Scan failed:", err)
		os.Exit(2)
	}
	if len(problems) == 0 {
		fmt.Printf("Checked %d Markdown files: no broken links found.\n", files)
		return
	}
	for _, item := range problems {
		fmt.Printf("%s:%d: %s - %s\n", item.file, item.line, item.target, item.reason)
	}
	fmt.Printf("Found %d broken link(s) in %d Markdown file(s).\n", len(problems), files)
	os.Exit(1)
}

func scanFile(root, path string, remote bool, client *http.Client) ([]problem, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	found := make([]problem, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		for _, match := range markdownLink.FindAllStringSubmatch(scanner.Text(), -1) {
			raw := strings.TrimSpace(match[1])
			if strings.HasPrefix(raw, "<") && strings.Contains(raw, ">") {
				raw = raw[1:strings.Index(raw, ">")]
			} else if fields := strings.Fields(raw); len(fields) > 0 {
				raw = strings.Trim(fields[0], "\"'")
			}
			target := strings.SplitN(raw, "#", 2)[0]
			if target == "" {
				continue
			}
			parsed, parseErr := url.Parse(target)
			if parseErr != nil {
				found = append(found, problem{filepath.ToSlash(path), lineNumber, target, "invalid link"})
				continue
			}
			if parsed.Scheme == "http" || parsed.Scheme == "https" {
				if remote {
					if reason := checkRemote(client, target); reason != "" {
						found = append(found, problem{filepath.ToSlash(path), lineNumber, target, reason})
					}
				}
				continue
			}
			if parsed.Scheme != "" {
				continue
			}
			localPath, unescapeErr := url.PathUnescape(parsed.Path)
			if unescapeErr != nil {
				found = append(found, problem{filepath.ToSlash(path), lineNumber, target, "invalid path encoding"})
				continue
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(localPath)))
			if !withinRoot(root, resolved) {
				found = append(found, problem{filepath.ToSlash(path), lineNumber, target, "target is outside the scanned folder"})
				continue
			}
			if _, statErr := os.Stat(resolved); statErr != nil {
				found = append(found, problem{filepath.ToSlash(path), lineNumber, target, "file does not exist"})
			}
		}
	}
	return found, scanner.Err()
}

func withinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func checkRemote(client *http.Client, target string) string {
	request, err := http.NewRequest(http.MethodHead, target, nil)
	if err != nil {
		return "invalid URL"
	}
	response, err := client.Do(request)
	if err != nil {
		return "request failed: " + err.Error()
	}
	response.Body.Close()
	if response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented {
		request, _ = http.NewRequest(http.MethodGet, target, nil)
		response, err = client.Do(request)
		if err != nil {
			return "request failed: " + err.Error()
		}
		response.Body.Close()
	}
	if response.StatusCode >= 400 {
		return fmt.Sprintf("HTTP %d", response.StatusCode)
	}
	return ""
}
