package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Runtime) buildValidationPrompt(draft generalDraft) (string, error) {
	summary, err := summarizeProjectContext(draft.WorkRoot)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(fmt.Sprintf(`You are validating a draft coding task before a new Telegram session topic is created.

Selected agent: %s
Working directory: %s
User request: %s

Project context:
%s

Rewrite the user request into a concise launch brief for the coding agent.
Rules:
- Keep the original intent.
- Use the project context to make the brief clearer and more grounded.
- Do not invent features the user did not ask for.
- Return only the launch brief, in plain text, 2-5 short sentences.`, draft.Provider, draft.WorkRoot, draft.Task, summary)), nil
}

func summarizeProjectContext(root string) (string, error) {
	root, err := ensureDirectory(root)
	if err != nil {
		return "", err
	}

	lines := []string{fmt.Sprintf("root: %s", root)}

	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("read root: %w", err)
	}

	dirs := make([]string, 0, len(entries))
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			dirs = append(dirs, name)
		} else {
			files = append(files, name)
		}
	}

	if len(dirs) > 0 {
		lines = append(lines, "dirs: "+strings.Join(trimList(dirs, 8), ", "))
	}
	if len(files) > 0 {
		lines = append(lines, "files: "+strings.Join(trimList(files, 8), ", "))
	}

	for _, name := range []string{"go.mod", "package.json", "README.md", "README", "pyproject.toml"} {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		snippet := strings.TrimSpace(string(data))
		if snippet == "" {
			continue
		}
		if len(snippet) > 280 {
			snippet = snippet[:280]
		}
		lines = append(lines, fmt.Sprintf("%s: %s", name, sanitizeSnippet(snippet)))
	}

	return strings.Join(lines, "\n"), nil
}

func trimList(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func sanitizeSnippet(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\n", " | ")
	return strings.Join(strings.Fields(value), " ")
}
