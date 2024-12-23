package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type PRDetails struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	PullNumber  int    `json:"pull_number"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type Comment struct {
	Path     string `json:"path"`
	Position int    `json:"position"`
	Body     string `json:"body"`
}

type Hunk struct {
	Header  string
	Content string
	Lines   []string
}

type ParsedFile struct {
	Path  string
	Hunks []Hunk
}

func getPRDetails() (*PRDetails, error) {
	eventPayload, err := os.ReadFile(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		return nil, err
	}

	var details PRDetails
	err = json.Unmarshal(eventPayload, &details)
	if err != nil {
		return nil, err
	}

	return &details, nil
}

func getDiff(owner, repo string, pullNumber int, githubToken string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, pullNumber)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+githubToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func parseDiff(diff string) ([]ParsedFile, error) {
	var files []ParsedFile
	var currentFile *ParsedFile
	var currentHunk *Hunk

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git"):
			if currentFile != nil {
				files = append(files, *currentFile)
			}
			currentFile = &ParsedFile{}

		case strings.HasPrefix(line, "--- a/"):
			if currentFile != nil {
				currentFile.Path = strings.TrimPrefix(line, "--- a/")
			}

		case strings.HasPrefix(line, "+++ b/"):
			if currentFile != nil {
				currentFile.Path = strings.TrimPrefix(line, "+++ b/")
			}

		case strings.HasPrefix(line, "@@"):
			if currentFile != nil {
				if currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
				}
				currentHunk = &Hunk{Header: line}
			}

		default:
			if currentHunk != nil {
				currentHunk.Lines = append(currentHunk.Lines, line)
				currentHunk.Content += line + "\n"
			}
		}
	}
	if currentFile != nil {
		files = append(files, *currentFile)
	}
	return files, nil
}

func createPrompt(file ParsedFile, hunk Hunk, title, description string) string {
	return fmt.Sprintf(`
Your task is to review pull requests. Instructions:
- Provide comments and suggestions ONLY if there is something to improve.
- Focus on bugs, security issues, and performance problems.
- Avoid generic comments and highlight critical issues.

File: %s
Pull Request Title: %s
Pull Request Description: %s

Diff Context:
%s
`, file.Path, title, description, hunk.Content)
}

func analyzeCodeUsingGemini(parsedFiles []ParsedFile, title, description, geminiApiKey string) ([]Comment, error) {
	modelName := os.Getenv("GEMINI_MODEL")
	if modelName == "" {
		modelName = "gemini-1.5-flash-002"
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(geminiApiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %v", err)
	}
	defer client.Close()

	model := client.GenerativeModel(modelName)
	var comments []Comment

	for _, file := range parsedFiles {
		for _, hunk := range file.Hunks {
			prompt := createPrompt(file, hunk, title, description)

			response, err := model.GenerateContent(ctx, genai.Text(prompt))
			if err != nil {
				return nil, fmt.Errorf("error analyzing code with Gemini: %v", err)
			}

			for _, candidate := range response.Candidates {
				if candidate.Content != nil {
					var fullText string
					for _, part := range candidate.Content.Parts {
						switch p := part.(type) {
						case genai.Text:
							// Handle text content
							fullText += p
						case genai.FunctionCall:
							// Handle function call
							fullText += fmt.Sprintf("[Function call: %s]", p.Name)
						case genai.ExecutableCode:
							// Handle executable code
							// Handle text content by accessing the field or method that contains the string
							fullText += p.Text // Replace "Text" with the actual field or method fmt.Sprintf("[Code: %s]", p.Code)
						case genai.CodeExecutionResult:
							// Handle code execution results
							fullText += fmt.Sprintf("[Execution result: %s]", p.Result)
						default:
							fmt.Printf("Unhandled part type: %T\n", part)
						}
					}

					comments = append(comments, Comment{
						Path:     file.Path,
						Position: 0, // Adjust based on the hunk line
						Body:     fullText,
					})
				}
			}
		}
	}
	return comments, nil
}

func postReviewComments(owner, repo string, pullNumber int, comments []Comment, githubToken string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/reviews", owner, repo, pullNumber)
	requestBody, _ := json.Marshal(map[string]interface{}{
		"body":     "Automated review by Gemini AI",
		"event":    "COMMENT",
		"comments": comments,
	})

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	req.Header.Set("Authorization", "Bearer "+githubToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("failed to post comments: %s", string(body))
	}

	return nil
}

func main() {
	githubToken := os.Getenv("INPUT_GITHUB_TOKEN")
	geminiApiKey := os.Getenv("INPUT_GEMINI_API_KEY")

	if githubToken == "" || geminiApiKey == "" {
		fmt.Println("Error: Missing required inputs INPUT_GITHUB_TOKEN or INPUT_GEMINI_API_KEY.")
		return
	}

	prDetails, err := getPRDetails()
	if err != nil {
		fmt.Println("Error fetching PR details:", err)
		return
	}

	diff, err := getDiff(prDetails.Owner, prDetails.Repo, prDetails.PullNumber, githubToken)
	if err != nil {
		fmt.Println("Error fetching diff:", err)
		return
	}

	parsedFiles, err := parseDiff(diff)
	if err != nil {
		fmt.Println("Error parsing diff:", err)
		return
	}

	comments, err := analyzeCodeUsingGemini(parsedFiles, prDetails.Title, prDetails.Description, geminiApiKey)
	if err != nil {
		fmt.Println("Error analyzing code:", err)
		return
	}

	err = postReviewComments(prDetails.Owner, prDetails.Repo, prDetails.PullNumber, comments, githubToken)
	if err != nil {
		fmt.Println("Error posting comments:", err)
		return
	}

	fmt.Println("Review comments posted successfully.")
}
