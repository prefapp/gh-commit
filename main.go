package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v67/github"
	"github.com/prefapp/gh-commit/git"
)

func main() {

	currentDir, err := os.Getwd()


	repo := flag.String("R","", "Repository to use")
	branch := flag.String("b", "main", "Branch to use")
	dir := flag.String("d", currentDir, "Directory to use")

	flag.Parse()

	if dir == nil && err != nil {
		fmt.Println("Error getting current directory:", err)
		return
	}

	host, _ := auth.DefaultHost()
	token, _ := auth.TokenForHost(host)
	
	client := github.NewClient(nil).WithAuthToken(token)

	parsedRepo, err := repository.Parse(*repo)

	if err != nil {
		panic(err)
	}

	// upload files
	ref, _, err := git.UploadToRepo(context.Background(), client, parsedRepo, *dir, *branch)

	if err != nil {
		fmt.Println("Error uploading files:", err)
		return
	}

	fmt.Println("Files uploaded to", *repo, "on branch", *branch, "with ref", *ref)

}
