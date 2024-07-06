package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

const BASE_API = "https://api.github.com"

type PullRequest struct {
	MergedAt       time.Time `json:"merged_at"`
	Title          string    `json:"title"`
	HtmlUrl        string    `json:"html_url"`
	Number         int       `json:"number"`
	MergeCommitSha string    `json:"merge_commit_sha"`
}

type Committer struct {
	Date time.Time `json:"date"`
}

type CommitData struct {
	Message   string    `json:"message"`
	Committer Committer `json:"committer"`
}

type CommitMeta struct {
	Sha     string     `json:"sha"`
	HtmlUrl string     `json:"html_url"`
	Commit  CommitData `json:"commit"`
}

type Result struct {
	Date  time.Time
	Value string
}

func pages(url string, process func([]byte) bool) {
	count := 0
	for {
		count += 1
		if count > 20 {
			fmt.Println("Too many pages")
			break
		}
		fmt.Println("Getting URL", url)
		request, err := http.NewRequest("GET", url, nil)
		if err != nil {
			panic(err)
		}

		if token, err := os.ReadFile("access_token.txt"); err == nil {
			request.Header.Add("Authorization", fmt.Sprintf("Bearer %v", string(token)))
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			panic(err)
		}
		defer response.Body.Close()

		data, err := io.ReadAll(response.Body)
		if err != nil {
			panic(err)
		}
		if !process(data) {
			break
		}

		linkHeader := response.Header.Get("link")
		splitNext := strings.Split(linkHeader, ">; rel=\"next\"")
		if len(splitNext) != 2 {
			fmt.Println("No next link", linkHeader)
			break
		}
		splitStart := strings.Split(splitNext[0], "<")
		if len(splitStart) < 2 {
			fmt.Println("No starting link", linkHeader)
			break
		}
		url = splitStart[len(splitStart)-1]
	}
}

func main() {
	commitShas := []string{}
	result := []Result{}
	current := time.Now()
	startTime := time.Date(current.Year(), ((current.Month()-2)/3)*3+1, 1, 0, 0, 0, 0, current.Location())
	endTime := time.Date(current.Year(), ((current.Month()-2)/3)*3+4, 1, 0, 0, 0, 0, current.Location())
	fmt.Println("Starting from", startTime)
	pages(fmt.Sprintf("%v/repos/graphiteeditor/graphite/pulls?state=closed", BASE_API), func(data []byte) bool {
		pullRequests := []PullRequest{}
		err := json.Unmarshal(data, &pullRequests)
		if err != nil {
			panic(err)
		}
		fmt.Println(pullRequests)
		anyValid := false
		for _, pullRequest := range pullRequests {
			if len(pullRequest.MergeCommitSha) < 2 {
				continue
			}
			if pullRequest.MergedAt.Before(startTime) {
				continue
			}
			anyValid = true
			if pullRequest.MergedAt.After(endTime) {
				continue
			}
			commitShas = append(commitShas, pullRequest.MergeCommitSha)
			value := fmt.Sprintf("- %v <small>([#%v](%v))</small>\n\n", pullRequest.Title, pullRequest.Number, pullRequest.HtmlUrl)
			result = append(result, Result{Value: value, Date: pullRequest.MergedAt})

		}
		return anyValid
	})

	pages(fmt.Sprintf("%v/repos/graphiteeditor/graphite/commits?since=%v&until=%v", BASE_API, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)), func(data []byte) bool {
		commits := []CommitMeta{}
		err := json.Unmarshal(data, &commits)
		if err != nil {
			panic(err)
		}
		fmt.Println(commits)
		for _, commit := range commits {
			if slices.Contains(commitShas, commit.Sha) {
				continue
			}

			hasPulls := false
			pages(fmt.Sprintf("%v/repos/graphiteeditor/graphite/commits/%v/pulls", BASE_API, commit.Sha), func(data []byte) bool {
				pulls := []any{}
				err := json.Unmarshal(data, &pulls)
				if err != nil {
					panic(err)
				}

				if len(pulls) > 0 {
					hasPulls = true
				}

				return false
			})
			if hasPulls {
				continue
			}

			value := fmt.Sprintf("- %v <small>([commit %v](%v))</small>\n\n", strings.Split(commit.Commit.Message, "\n")[0], commit.Sha[:7], commit.HtmlUrl)
			result = append(result, Result{Value: value, Date: commit.Commit.Committer.Date})
		}
		return true
	})

	slices.SortFunc(result, func(a Result, b Result) int {
		if a.Date.Before(b.Date) {
			return -1
		} else if b.Date.Before(a.Date) {
			return 1
		} else {
			return 0
		}
	})

	resultString := ""
	for _, item := range result {
		resultString += item.Value
	}

	os.WriteFile("output.md", []byte(resultString), 0644)
}
