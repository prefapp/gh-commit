package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v67/github"
	"github.com/prefapp/gh-commit/git"
)

func main() {

	currentDir, err := os.Getwd()

	repo := flag.String("R", "", "Repository to use")
	branch := flag.String("b", "main", "Branch to use")
	dir := flag.String("d", currentDir, "Directory to use")
	message := flag.String("m", "Commit message", "Commit message")
	deletePath := flag.String("delete-path", "", "Path in the origin repository to delete files from before adding new ones")
	headBranch := flag.String("h", "", "Head branch name")
	flag.Parse()

	if *headBranch == "" {
		*headBranch, err = git.GetHeadBranch(*dir)

		if err != nil {
			panic(err)
		}
	}

	if dir == nil && err != nil {
		fmt.Println("Error getting current directory:", err)
		return
	}

	host, _ := auth.DefaultHost()
	token, _ := auth.TokenForHost(host)

	rateLimiter, err := github_ratelimit.NewRateLimitWaiterClient(nil)
	if err != nil {
		panic(err)
	}

	client := github.NewClient(rateLimiter).WithAuthToken(token)

	parsedRepo, err := repository.Parse(*repo)

	if err != nil {
		panic(err)
	}

	// upload files
	ref, _, err := git.UploadToRepo(context.Background(), client, parsedRepo, *dir, *deletePath, *branch, *headBranch, *message)

	if err != nil {
		fmt.Println("Error uploading files:", err)
		return
	}

	fmt.Println("Files uploaded to", *repo, "on branch", *branch, "with ref", *ref)

}
