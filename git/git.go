package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	})
}

func createNewTree(ctx context.Context, client *github.Client, repo repository.Repository, blobs []*github.Blob, blobPaths []string, parentTreeSha string) (*github.Tree, *github.Response, error) {

	tree := []*github.TreeEntry{}

	for i, blob := range blobs {
		treeEntry := &github.TreeEntry{
			Path: &blobPaths[i],
			Mode: github.String("100644"),
			Type: github.String("blob"),
		}

		if blob == nil {

			continue

		}

		if blob.SHA != nil {
			treeEntry.SHA = blob.SHA
		}
		tree = append(tree, treeEntry)
	}

	return client.Git.CreateTree(ctx, repo.Owner, repo.Name, parentTreeSha, tree)
}

func createNewCommit(ctx context.Context, client *github.Client, repo repository.Repository, currentTree *github.Tree, parentCommit *github.RepositoryCommit, message string) (*github.Commit, *github.Response, error) {

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
	}, true)
}

func listFilesOrigin(ctx context.Context, client *github.Client, repo repository.Repository, branch string) ([]string, error) {

	// Get the current commit
	currentCommit, err := getCurrentCommit(ctx, client, repo, branch)
	if err != nil {
		return nil, err
	}

	// Get the tree
	tree, _, err := client.Git.GetTree(ctx, repo.Owner, repo.Name, *currentCommit.Commit.Tree.SHA, true)
	// If there is an error here most likely the tree is not found and the repository is empty
	if err != nil {
		return []string{}, nil
	}

	// Get the files
	files := []string{}
	for _, entry := range tree.Entries {
		files = append(files, *entry.Path)
	}

	return files, nil
}

func getDeletedFiles(basePath string, originFiles []string, deletedPath string, updatedFiles []string) []string {

	files := []string{}
	fmt.Println("--- Origin files--")
	fmt.Println(originFiles)
	for _, f := range originFiles {
		if deletedPath == "" && !utils.FileExistsInList(updatedFiles, filepath.Join(basePath, f)) && (strings.HasSuffix(f, ".yml") || strings.HasSuffix(f, ".yaml")) {
			files = append(files, f)
			continue
		}
		if strings.HasPrefix(f, deletedPath) && !utils.FileExistsInList(updatedFiles, filepath.Join(basePath, f)) {
			files = append(files, f)
			continue
		}
	}

	return files

}

func UploadToRepo(ctx context.Context, client *github.Client, repo repository.Repository, path string, deletePath string, branch string, message string) (*github.Reference, *github.Response, error) {

	// Get the current currentCommit
	currentCommit, err := getCurrentCommit(ctx, client, repo, branch)
	if err != nil {
		panic(err)
	}

	// List all files in the path
	files := utils.ListFiles(path, []string{".git"})
	fmt.Println("--- Files--")
	fmt.Println(files)
	// Get a list of deleted files, this means that the files that are in the origin repository inspecting the tree but in the files list
	// are not present
	originFiles, err := listFilesOrigin(ctx, client, repo, branch)

	if err != nil {
		return nil, nil, err
	}

	// Create a blob for each file
	blobs := []*github.Blob{}
	blobPaths := []string{}

	// Delete the files
	// Get the files that are deleted
	deletedFiles := getDeletedFiles(path, originFiles, deletePath, files)
	fmt.Println("--- Deleted files--")
	fmt.Println(deletedFiles)

	for _, f := range deletedFiles {
		blobs = append(blobs, &github.Blob{
			SHA: nil,
		})
		blobPaths = append(blobPaths, f)
	}

	// Get the updated files
	fmt.Println("--- Updated files--")
	fmt.Println(files)

	for _, file := range files {
		blob, _, _ := createBlobForFile(ctx, client, repo, file)

		blobs = append(blobs, blob)
		relativePath, err := filepath.Rel(path, file)

		if err != nil {
			return nil, nil, err
		}
		blobPaths = append(blobPaths, relativePath)
	}

	tree, _, err := createNewTree(ctx, client, repo, blobs, blobPaths, currentCommit.GetSHA())
	if err != nil {
		return nil, nil, err
	}

	commit, _, err := createNewCommit(ctx, client, repo, tree, currentCommit, message)
	if err != nil {
		return nil, nil, err
	}

	return setBranchToCommit(ctx, client, repo, branch, commit)

}
