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
	defaultRepo, err := repository.Current()

	repo := flag.String("R", fmt.Sprintf("%s/%s", defaultRepo.Owner, defaultRepo.Name), "Repository to use")
	branch := flag.String("b", "main", "Branch to use")
	dir := flag.String("d", currentDir, "Directory to use")
	message := flag.String("m", "Commit message", "Commit message")
	deletePath := flag.String("delete-path", "", "Path in the origin repository to delete files from before adding new ones")
	baseBranch := flag.String("base", "", "Base branch name")
	createEmpty := flag.Bool("e", false, "Create an empty commit")
	allowEmpty := flag.Bool("a", false, "Allow empty commits")
	flag.Parse()

	if *baseBranch == "" {
		*baseBranch, err = git.GetBaseBranch(*dir)

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
	ref, _, err, exitCode := git.UploadToRepo(
		context.Background(), client, parsedRepo,
		*dir, *deletePath, *branch, *baseBranch,
		*message, createEmpty, allowEmpty,
	)

	if err != nil {
		fmt.Println("Error uploading files:", err)
	} else {
		fmt.Println("Files uploaded to", *repo, "on branch", *branch, "with ref", *ref)
	}

	os.Exit(exitCode)

}
