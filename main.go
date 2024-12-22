package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
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

func analyzeCode(diff, title, description, geminiApiKey string) ([]Comment, error) {
	prompt := fmt.Sprintf(
		"Review and improve the following code:\n\nTitle: %s\nDescription: %s\nDiff:\n%s\n\nTasks:\n1. Review the code and provide suggestions.\n2. Identify areas to improve the code's structure and robustness.\n3. Highlight potential security issues and suggest fixes.",
		title, description, diff)

	url := "https://gemini.api/endpoint"

	requestBody, _ := json.Marshal(map[string]string{
		"prompt": prompt,
	})

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	req.Header.Set("Authorization", "Bearer "+geminiApiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var aiResponse struct {
		Comments []Comment `json:"comments"`
	}
	err = json.NewDecoder(resp.Body).Decode(&aiResponse)
	if err != nil {
		return nil, err
	}

	return aiResponse.Comments, nil
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
		fmt.Println("Error: Missing required inputs GITHUB_TOKEN or GEMINI_API_KEY.")
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

	comments, err := analyzeCode(diff, prDetails.Title, prDetails.Description, geminiApiKey)
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
