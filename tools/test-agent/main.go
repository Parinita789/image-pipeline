//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	anthropicAPI = "https://api.anthropic.com/v1/messages"
	claudeModel  = "claude-sonnet-4-20250514"
	maxTokens    = 4096
	maxRetries   = 3
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type APIRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"message"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type APIResponse struct {
	Content []ContentBlock `json:"content"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func callClaude(prompt string) (string, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	body, _ := json.Marshal(APIRequest{
		Model:     claudeModel,
		MaxTokens: maxTokens,
		Messages:  []Message{{Role: "user", Content: prompt}},
	})

	req, _ := http.NewRequest("POST", anthropicAPI, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var apiResp APIResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse API response: %w\nraw: %s", err, raw)
	}
	if apiResp.Error != nil {
		return "", fmt.Errorf("API error: %s", apiResp.Error.Message)
	}
	if len(apiResp.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}
	return apiResp.Content[0].Text, nil
}

func runCmd(name string, args ...string) (string, string, int) {
	cmd := exec.Command(name, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	code := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		} else {
			code = 1
		}
	}
	return out.String(), errOut.String(), code
}

func getChangedGoFiles() []string {
	stdout, _, code := runCmd("git", "diff", "HEAD~1", "HEAD", "--name-only")
	if code != 0 {
		stdout, _, _ = runCmd("git", "diff", "--cached", "--name-only")
	}

	var files []string
	for _, f := range strings.Split(strings.TrimSpace(stdout), "\n") {
		f = strings.TrimSpace(f)
		if strings.HasSuffix(f, ".go") &&
			!strings.HasSuffix(f, "_test.go") &&
			!strings.Contains(f, "tools/test-agent") &&
			f != "" {
			files = append(files, f)
		}
	}
	return files
}

func testFileFor(src string) string {
	return strings.TrimSuffix(src, ".go") + "_test.go"
}

func readFileSafe(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(b)
}

func cleanFences(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"```go", "```"} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimPrefix(s, prefix)
			break
		}
	}
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func generateTests(srcFile, srcCode, existingTests string) (string, error) {
	var testSection string
	if existingTests != "" {
		testSection = fmt.Sprintf("Existing test file:\n```go\n%s\n```", existingTests)
	} else {
		testSection = "No test file exists yet — create one from scratch."
	}

	prompt := fmt.Sprintf(`You are an expert Go test engineer working inside an existing Go project.
 
Your job:
1. Fix any broken or outdated test cases
2. Add missing tests for untested functions, edge cases, and error paths
3. Use table-driven tests where appropriate
4. Use only the standard "testing" package unless testify is already imported
5. Preserve the existing package name and imports
6. Return ONLY the raw Go test file — no markdown fences, no explanation
 
Source file: %s
`+"```go\n%s\n```"+`
 
%s
 
Return only the complete updated _test.go file:`,
		srcFile, srcCode, testSection)

	return callClaude(prompt)
}

func fixTests(srcFile, testFile, srcCode, testCode, failOutput string, attempt int) (string, error) {
	prompt := fmt.Sprintf(`You are a Go test engineer fixing failing tests. This is attempt %d of %d.
 
Source file (%s):
`+"```go\n%s\n```"+`
 
Current test file (%s):
`+"```go\n%s\n```"+`
 
Test failure output:
`+"```\n%s\n```"+`
 
Fix ALL issues. Return only the corrected test file with no markdown or explanation:`,
		attempt, maxRetries,
		srcFile, srcCode,
		testFile, testCode,
		failOutput)

	return callClaude(prompt)
}

func agenticLoop(srcFile string) (testFile string, changed bool, err error) {
	srcCode := readFileSafe(srcFile)
	if srcCode == "" {
		return "", false, fmt.Errorf("cannot read %s", srcFile)
	}

	testFile = testFileFor(srcFile)
	existingTests := readFileSafe(testFile)

	// Step 1: Generate / update tests
	fmt.Printf("  🤖 Generating tests for %s\n", srcFile)
	updated, err := generateTests(srcFile, srcCode, existingTests)
	if err != nil {
		return testFile, false, fmt.Errorf("generation failed: %w", err)
	}
	updated = cleanFences(updated)

	if err := os.WriteFile(testFile, []byte(updated), 0644); err != nil {
		return testFile, false, fmt.Errorf("write failed: %w", err)
	}
	fmt.Printf("  ✅ Wrote %s\n", testFile)

	// Self-correcting run loop
	pkg := "./" + filepath.Dir(srcFile) + "/..."

	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("  🧪 Running tests (attempt %d/%d): %s\n", attempt, maxRetries, pkg)
		out, errOut, code := runCmd("go", "test", "-v", "-count=1", pkg)
		combined := out + errOut

		if code == 0 {
			fmt.Printf("  ✅ All tests passed!\n")
			return testFile, true, nil
		}

		fmt.Printf("  ❌ Tests failed (attempt %d)\n", attempt)
		if attempt == maxRetries {
			fmt.Printf("  ⚠️  Max retries reached. Saving best effort.\n")
			fmt.Printf("--- test output ---\n%s\n---\n", combined)
			return testFile, true, nil // still write the file; let CI show the failure
		}

		fmt.Printf("  🔧 Asking Claude to fix failures...\n")
		current := readFileSafe(testFile)
		fixed, err := fixTests(srcFile, testFile, srcCode, current, combined, attempt+1)
		if err != nil {
			return testFile, true, fmt.Errorf("fix failed: %w", err)
		}
		if err := os.WriteFile(testFile, []byte(cleanFences(fixed)), 0644); err != nil {
			return testFile, true, fmt.Errorf("write failed: %w", err)
		}
	}

	return testFile, true, nil
}

func commitDirect(files []string) {
	runCmd("git", "config", "user.name", "AI Test Agent")
	runCmd("git", "config", "user.email", "ai-agent@noreply.github.com")
	for _, f := range files {
		runCmd("git", "add", f)
	}
	_, _, code := runCmd("git", "diff", "--cached", "--quiet")
	if code == 0 {
		fmt.Println("No changes to commit.")
		return
	}
	runCmd("git", "commit", "-m", "🤖 AI: fix/add test cases")
	runCmd("git", "push")
	fmt.Println("✅ Committed and pushed.")
}

func openPR(files []string) {
	branch := "ai-test-agent/update-tests"
	runCmd("git", "checkout", "-b", branch)
	runCmd("git", "config", "user.name", "AI Test Agent")
	runCmd("git", "config", "user.email", "ai-agent@noreply.github.com")
	for _, f := range files {
		runCmd("git", "add", f)
	}
	_, _, code := runCmd("git", "diff", "--cached", "--quiet")
	if code == 0 {
		fmt.Println("No changes to commit.")
		return
	}
	runCmd("git", "commit", "-m", "🤖 AI: fix/add test cases")
	runCmd("git", "push", "--set-upstream", "origin", branch)

	body := "## 🤖 AI Test Agent\n\nUpdated test files:\n"
	for _, f := range files {
		body += fmt.Sprintf("- `%s`\n", f)
	}
	stdout, stderr, c := runCmd("gh", "pr", "create",
		"--title", "🤖 AI Test Agent: fix/add tests",
		"--body", body,
		"--base", "main",
	)
	if c != 0 {
		fmt.Printf("⚠️  PR creation failed: %s %s\n", stdout, stderr)
		return
	}
	fmt.Printf("🎉 PR created: %s\n", strings.TrimSpace(stdout))
}

func main() {
	// AGENT_MODE: "commit" (default) or "pr"
	mode := os.Getenv("AGENT_MODE")
	if mode == "" {
		mode = "commit"
	}

	fmt.Println("🤖 AI Test Agent starting...")

	changedFiles := getChangedGoFiles()
	if len(changedFiles) == 0 {
		fmt.Println("✅ No Go source files changed. Nothing to do.")
		return
	}
	fmt.Printf("📂 Changed files: %v\n\n", changedFiles)

	var updatedTests []string
	for _, src := range changedFiles {
		fmt.Printf("🔍 %s\n", src)
		testFile, changed, err := agenticLoop(src)
		if err != nil {
			fmt.Printf("  ⚠️  Skipped due to error: %v\n", err)
			continue
		}
		if changed {
			updatedTests = append(updatedTests, testFile)
		}
	}

	if len(updatedTests) == 0 {
		fmt.Println("No test files were updated.")
		return
	}

	fmt.Printf("\n📝 Updated: %v\n", updatedTests)

	if mode == "pr" {
		openPR(updatedTests)
	} else {
		commitDirect(updatedTests)
	}

	fmt.Println("\n🎉 Done!")
}
