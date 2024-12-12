package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v67/github"
	"github.com/prefapp/gh-commit/utils"
)

func getCurrentCommit(ctx context.Context, client *github.Client, repo repository.Repository, branch string) (*github.RepositoryCommit, error) {
	
	// Get the branch reference
	branchRef, _, err := client.Git.GetRef(ctx, repo.Owner, repo.Name, fmt.Sprintf("heads/%s", branch))

	if err != nil {
		return nil, err
	}

	// Get the commit SHA
	commitSha := branchRef.Object.SHA

	// Get the commit
	commit, _, err := client.Repositories.GetCommit(ctx, repo.Owner, repo.Name, *commitSha, nil)

	if err != nil {
		return nil, err
	}

	return commit, nil

}

func createBlobForFile(ctx context.Context, client *github.Client, repo repository.Repository, file string) (*github.Blob, *github.Response, error) {
	
	// Read the file content
	content, err := os.ReadFile(file)

	if err != nil {
		return nil, nil, err
	}

	return client.Git.CreateBlob(ctx, repo.Owner, repo.Name, &github.Blob{
		Content:  github.String(string(content)),
		Encoding: github.String("utf-8"),
})}

func createNewTree(ctx context.Context, client *github.Client, repo repository.Repository, blobs []*github.Blob, blobPaths []string, parentTreeSha string) (*github.Tree, *github.Response, error) {
	
	tree := []*github.TreeEntry{}

	for i, blob := range blobs {
		tree = append(tree, &github.TreeEntry{
			Path: &blobPaths[i],
			Mode: github.String("100644"),
			Type: github.String("blob"),
			SHA:  blob.SHA,
		})
	}

	return client.Git.CreateTree(ctx, repo.Owner, repo.Name, parentTreeSha, tree)
}

func createNewCommit(ctx context.Context, client *github.Client, repo repository.Repository,currentTree *github.Tree, parentCommit *github.RepositoryCommit, message string) (*github.Commit, *github.Response, error) {
	
	commit := &github.Commit{
		Message: github.String(message),
		Tree:    &github.Tree{SHA: github.String(currentTree.GetSHA())},
		Parents: []*github.Commit{
			{
				SHA: github.String(parentCommit.GetSHA()),
			},
		},
	}
	return client.Git.CreateCommit(ctx, repo.Owner, repo.Name, commit, nil)
}

func setBranchToCommit(ctx context.Context, client *github.Client, repo repository.Repository, branch string, commit *github.Commit) (*github.Reference, *github.Response, error) {
	
	ref := fmt.Sprintf("refs/heads/%s", branch)

	return client.Git.UpdateRef(ctx, repo.Owner, repo.Name, &github.Reference{
		Ref: github.String(ref),
		Object: &github.GitObject{
			SHA: commit.SHA,
		},
	}, false)
}

func UploadToRepo(ctx context.Context,client *github.Client, repo repository.Repository, path string, branch string) (*github.Reference, *github.Response, error) {
	
	// Get the current currentCommit
	currentCommit, err := getCurrentCommit(ctx, client, repo, branch)
	if err != nil {
		panic(err)
	}


	// List all files in the path
	files := utils.ListFiles(path, []string{".git"})


	// Create a blob for each file
	blobs := []*github.Blob{}
	blobPaths := []string{}

	for _, file := range files {
		blob, _, _ := createBlobForFile(ctx, client, repo, file)

		blobs = append(blobs, blob)
		relativePath, err := filepath.Rel(path, file)

		// Debug log the relative path
		fmt.Printf("Relative path: %s\n", relativePath)

		if err != nil {
			return nil, nil, err
		}
		blobPaths = append(blobPaths, relativePath)
	}

	tree, _, err := createNewTree(ctx, client, repo, blobs, blobPaths, currentCommit.GetSHA())
	if err != nil {
		return nil, nil, err
	}

	commit, _, err := createNewCommit(ctx, client, repo, tree, currentCommit, "Foo")
	if err != nil {
		return nil, nil, err
	}

	return setBranchToCommit(ctx, client, repo, branch, commit)

}
