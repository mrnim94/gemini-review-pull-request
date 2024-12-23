package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

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

// PRDetails struct to hold pull request details
type PRDetails struct {
	Owner       string
	Repo        string
	PullNumber  int
	Title       string
	Description string
}

// GetPRDetails retrieves details of the pull request from GitHub Actions event payload
func GetPRDetails() (*PRDetails, error) {
	// Get the path to the GitHub event file
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return nil, errors.New("GITHUB_EVENT_PATH environment variable is not set")
	}

	// Open and read the event file
	file, err := os.Open(eventPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open event file: %v", err)
	}
	defer file.Close()

	var eventData map[string]interface{}
	if err := json.NewDecoder(file).Decode(&eventData); err != nil {
		return nil, fmt.Errorf("failed to decode JSON from event file: %v", err)
	}

	// Determine if the event was triggered by a comment on a PR or a direct PR event
	var pullNumber int
	var repoFullName string

	if issue, ok := eventData["issue"].(map[string]interface{}); ok {
		if prData, exists := issue["pull_request"].(map[string]interface{}); exists && prData != nil {
			// For comment triggers
			if number, ok := issue["number"].(float64); ok {
				pullNumber = int(number)
			} else {
				return nil, errors.New("invalid pull request number in issue payload")
			}
		} else {
			return nil, errors.New("issue payload does not contain pull_request data")
		}
		repoFullName = getRepoFullName(eventData)
	} else {
		// For direct PR events
		if number, ok := eventData["number"].(float64); ok {
			pullNumber = int(number)
		} else {
			return nil, errors.New("invalid pull request number in event payload")
		}
		repoFullName = getRepoFullName(eventData)
	}

	if repoFullName == "" {
		return nil, errors.New("repository full name not found in event data")
	}

	owner, repo, err := splitRepoFullName(repoFullName)
	if err != nil {
		return nil, err
	}

	title, description := getPRTitleAndDescription(eventData)

	return &PRDetails{
		Owner:       owner,
		Repo:        repo,
		PullNumber:  pullNumber,
		Title:       title,
		Description: description,
	}, nil
}

// Helper to extract repo full name from event data
func getRepoFullName(eventData map[string]interface{}) string {
	if repoData, ok := eventData["repository"].(map[string]interface{}); ok {
		if fullName, ok := repoData["full_name"].(string); ok {
			return fullName
		}
	}
	return ""
}

// Helper to split the repo full name into owner and repo
func splitRepoFullName(fullName string) (string, string, error) {
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository full name: %s", fullName)
	}
	return parts[0], parts[1], nil
}

// Helper to extract PR title and description
func getPRTitleAndDescription(eventData map[string]interface{}) (string, string) {
	if pullRequest, ok := eventData["pull_request"].(map[string]interface{}); ok {
		title := ""
		description := ""
		if t, ok := pullRequest["title"].(string); ok {
			title = t
		}
		if d, ok := pullRequest["body"].(string); ok {
			description = d
		}
		return title, description
	}
	return "No Title", "No Description"
}

// Helper function to load event data from the GITHUB_EVENT_PATH
func loadEventData() (map[string]interface{}, error) {
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return nil, fmt.Errorf("GITHUB_EVENT_PATH environment variable is not set")
	}

	file, err := os.Open(eventPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open event file: %v", err)
	}
	defer file.Close()

	var eventData map[string]interface{}
	if err := json.NewDecoder(file).Decode(&eventData); err != nil {
		return nil, fmt.Errorf("failed to decode JSON from event file: %v", err)
	}

	return eventData, nil
}

// Helper function to get the GITHUB_EVENT_NAME environment variable
func getEventName() string {
	return os.Getenv("GITHUB_EVENT_NAME")
}

//func getDiff(owner, repo string, pullNumber int, githubToken string) (string, error) {
//	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, pullNumber)
//	req, _ := http.NewRequest("GET", url, nil)
//	req.Header.Set("Authorization", "Bearer "+githubToken)
//
//	client := &http.Client{}
//	resp, err := client.Do(req)
//	if err != nil {
//		return "", err
//	}
//	defer resp.Body.Close()
//
//	body, err := ioutil.ReadAll(resp.Body)
//	if err != nil {
//		return "", err
//	}
//
//	return string(body), nil
//}
//
//func parseDiff(diff string) ([]ParsedFile, error) {
//	var files []ParsedFile
//	var currentFile *ParsedFile
//	var currentHunk *Hunk
//
//	lines := strings.Split(diff, "\n")
//	for _, line := range lines {
//		switch {
//		case strings.HasPrefix(line, "diff --git"):
//			if currentFile != nil {
//				files = append(files, *currentFile)
//			}
//			currentFile = &ParsedFile{}
//
//		case strings.HasPrefix(line, "--- a/"):
//			if currentFile != nil {
//				currentFile.Path = strings.TrimPrefix(line, "--- a/")
//			}
//
//		case strings.HasPrefix(line, "+++ b/"):
//			if currentFile != nil {
//				currentFile.Path = strings.TrimPrefix(line, "+++ b/")
//			}
//
//		case strings.HasPrefix(line, "@@"):
//			if currentFile != nil {
//				if currentHunk != nil {
//					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
//				}
//				currentHunk = &Hunk{Header: line}
//			}
//
//		default:
//			if currentHunk != nil {
//				currentHunk.Lines = append(currentHunk.Lines, line)
//				currentHunk.Content += line + "\n"
//			}
//		}
//	}
//	if currentFile != nil {
//		files = append(files, *currentFile)
//	}
//	return files, nil
//}
//
//func createPrompt(file ParsedFile, hunk Hunk, title, description string) string {
//	return fmt.Sprintf(`
//Your task is to review pull requests. Instructions:
//- Provide comments and suggestions ONLY if there is something to improve.
//- Focus on bugs, security issues, and performance problems.
//- Avoid generic comments and highlight critical issues.
//
//File: %s
//Pull Request Title: %s
//Pull Request Description: %s
//
//Diff Context:
//%s
//`, file.Path, title, description, hunk.Content)
//}
//
//func analyzeCodeUsingGemini(parsedFiles []ParsedFile, title, description, geminiApiKey string) ([]Comment, error) {
//	modelName := os.Getenv("GEMINI_MODEL")
//	if modelName == "" {
//		modelName = "gemini-1.5-flash-002"
//	}
//
//	ctx := context.Background()
//	client, err := genai.NewClient(ctx, option.WithAPIKey(geminiApiKey))
//	if err != nil {
//		return nil, fmt.Errorf("failed to create Gemini client: %v", err)
//	}
//	defer client.Close()
//
//	model := client.GenerativeModel(modelName)
//	var comments []Comment
//
//	for _, file := range parsedFiles {
//		for _, hunk := range file.Hunks {
//			prompt := createPrompt(file, hunk, title, description)
//
//			response, err := model.GenerateContent(ctx, genai.Text(prompt))
//			if err != nil {
//				return nil, fmt.Errorf("error analyzing code with Gemini: %v", err)
//			}
//
//			for _, candidate := range response.Candidates {
//				if candidate.Content != nil {
//					var fullText string
//					for _, part := range candidate.Content.Parts {
//						switch p := part.(type) {
//						case genai.Text:
//							// Handle text content
//							fullText += string(p)
//						case genai.FunctionCall:
//							// Handle function call
//							fullText += fmt.Sprintf("[Function call: %s]", p.Name)
//						case genai.ExecutableCode:
//							// Handle executable code
//							fullText += fmt.Sprintf("[Code: %s]", p.Code)
//						case genai.CodeExecutionResult:
//							// Handle code execution results
//							fullText += fmt.Sprintf("[Execution result: Outcome=%s, Output=%s]", p.Outcome, p.Output)
//						default:
//							fmt.Printf("Unhandled part type: %T\n", part)
//						}
//					}
//
//					comments = append(comments, Comment{
//						Path:     file.Path,
//						Position: 0, // Adjust based on the hunk line
//						Body:     fullText,
//					})
//				}
//			}
//		}
//	}
//	return comments, nil
//}
//
//func postReviewComments(owner, repo string, pullNumber int, comments []Comment, githubToken string) error {
//	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/reviews", owner, repo, pullNumber)
//	requestBody, _ := json.Marshal(map[string]interface{}{
//		"body":     "Automated review by Gemini AI",
//		"event":    "COMMENT",
//		"comments": comments,
//	})
//
//	// Log the URL and payload for debugging
//	fmt.Printf("Request URL: %s\n", url)
//	fmt.Printf("Request Body: %s\n", string(requestBody))
//
//	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
//	req.Header.Set("Authorization", "Bearer "+githubToken)
//	req.Header.Set("Content-Type", "application/json")
//
//	client := &http.Client{}
//	resp, err := client.Do(req)
//	if err != nil {
//		return err
//	}
//	defer resp.Body.Close()
//
//	if resp.StatusCode != http.StatusCreated {
//		body, _ := ioutil.ReadAll(resp.Body)
//		return fmt.Errorf("failed to post comments: %s", string(body))
//	}
//
//	return nil
//}

func main() {
	githubToken := os.Getenv("INPUT_GITHUB_TOKEN")
	geminiApiKey := os.Getenv("INPUT_GEMINI_API_KEY")

	if githubToken == "" || geminiApiKey == "" {
		fmt.Println("Error: Missing required inputs INPUT_GITHUB_TOKEN or INPUT_GEMINI_API_KEY.")
		return
	}

	prDetails, err := GetPRDetails()
	if err != nil {
		fmt.Printf("Error retrieving PR details: %v\n", err)
		return
	}

	fmt.Printf("PR Details: %+v\n", prDetails)

	// Load the event data
	eventData, err := loadEventData()
	if err != nil {
		fmt.Printf("Error loading event data: %v\n", err)
		return
	}

	// Get the event name
	eventName := getEventName()
	if eventName == "" {
		fmt.Println("GITHUB_EVENT_NAME is not set")
		return
	}

	fmt.Printf("Event Name: %s\n", eventName)
	fmt.Printf("Event Data: %+v\n", eventData)

	//diff, err := getDiff(prDetails.Owner, prDetails.Repo, prDetails.PullNumber, githubToken)
	//if err != nil {
	//	fmt.Println("Error fetching diff:", err)
	//	return
	//}
	//
	//parsedFiles, err := parseDiff(diff)
	//if err != nil {
	//	fmt.Println("Error parsing diff:", err)
	//	return
	//}
	//
	//comments, err := analyzeCodeUsingGemini(parsedFiles, prDetails.Title, prDetails.Description, geminiApiKey)
	//if err != nil {
	//	fmt.Println("Error analyzing code:", err)
	//	return
	//}
	//
	//err = postReviewComments(prDetails.Owner, prDetails.Repo, prDetails.PullNumber, comments, githubToken)
	//if err != nil {
	//	fmt.Println("Error posting comments:", err)
	//	return
	//}
	//
	//fmt.Println("Review comments posted successfully.")
}
