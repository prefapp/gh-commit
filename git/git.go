package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/go-git/go-git/v6"
	"github.com/google/go-github/v67/github"
)

func getGitPorcelain(dirPath string) (git.Status, error) {
	repo, err := git.PlainOpen(dirPath)
	if err != nil {
		return nil, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := wt.Status()
	if err != nil {
		return nil, err
	}
	return status, nil
}

func GetHeadBranch(repoPath string) (string, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", err
	}

	head, err := repo.Head()
	if err != nil {
		return "", err
	}
	headBranchName := strings.Split(head.Name().String(), "/")[2]

	return headBranchName, nil
}

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

	refExists, _, _ := client.Git.GetRef(ctx, repo.Owner, repo.Name, ref)

	if refExists == nil {
		return client.Git.CreateRef(ctx, repo.Owner, repo.Name, &github.Reference{
			Ref: github.String(ref),
			Object: &github.GitObject{
				SHA: commit.SHA,
			},
		})
	} else {
		return client.Git.UpdateRef(ctx, repo.Owner, repo.Name, &github.Reference{
			Ref: github.String(ref),
			Object: &github.GitObject{
				SHA: commit.SHA,
			},
		}, true)
	}

}

func getGroupedFiles(fileStatuses git.Status) ([]string, []string, []string, error) {
	addedFiles := []string{}
	updatedFiles := []string{}
	deletedFiles := []string{}

	for file, status := range fileStatuses {
		switch fmt.Sprintf("%c", status.Worktree) {
		case "D":
			deletedFiles = append(deletedFiles, file)
		case "M", "R", "C", "U":
			updatedFiles = append(updatedFiles, file)
		case "?", "A":
			addedFiles = append(addedFiles, file)
		default:
			return nil, nil, nil, errors.New(
				fmt.Sprintf(
					"Unsupported status code %c for file %s",
					status.Worktree,
					file,
				),
			)
		}
	}

	return addedFiles, updatedFiles, deletedFiles, nil
}

func UploadToRepo(
	ctx context.Context,
	client *github.Client,
	repo repository.Repository,
	path string,
	deletePath string,
	branch string,
	headBranch string,
	message string,
	createEmpty *bool,
	allowEmpty *bool,
) (*github.Reference, *github.Response, error) {

	// Get the current currentCommit
	currentCommit, err := getCurrentCommit(ctx, client, repo, headBranch)
	if err != nil {
		return nil, nil, err
	}

	fileStatuses, err := getGitPorcelain(path)

	addedFiles, updatedFiles, deletedFiles, err := getGroupedFiles(fileStatuses)
	if err != nil {
		return nil, nil, err
	}
	addedAndUpdatedFiles := append(updatedFiles, addedFiles...)

	if (len(addedAndUpdatedFiles) == 0 && len(deletedFiles) == 0 && *allowEmpty) || *createEmpty {
		// In order to push an empty commit, we first need to create a
		// dummy file and commit it to the branch
		dummy, err := os.CreateTemp("", "firestartr-empty-commit-dummy-*.txt")
		if err != nil {
			return nil, nil, err
		}
		defer func() {
			dummy.Close()
			os.Remove(dummy.Name())
		}()
		fileName := dummy.Name()

		blob, _, err := createBlobForFile(ctx, client, repo, fileName)
		if err != nil {
			return nil, nil, err
		}
		tree, _, err := createNewTree(
			ctx,
			client,
			repo,
			[]*github.Blob{blob},
			[]string{fileName},
			currentCommit.GetSHA(),
		)
		if err != nil {
			return nil, nil, err
		}

		commit, _, err := createNewCommit(
			ctx, client, repo, tree,
			currentCommit, message,
		)
		if err != nil {
			return nil, nil, err
		}

		// Then we delete it and set that commit as the new head commit
		// of the branch. This results in a commit that has no changes
		// with the main branch, but a different hash
		emptyTree, _, err := createNewTree(
			ctx,
			client,
			repo,
			[]*github.Blob{{SHA: nil}},
			[]string{fileName},
			commit.GetSHA(),
		)
		if err != nil {
			return nil, nil, err
		}

		emptyCommit, _, err := createNewCommit(
			ctx, client, repo, emptyTree,
			currentCommit, message,
		)
		if err != nil {
			return nil, nil, err
		}

		ref, resp, respErr := setBranchToCommit(ctx, client, repo, branch, emptyCommit)

		err = os.Remove(fileName)
		if err != nil {
			return nil, nil, err
		}

		return ref, resp, respErr
	} else {
		if len(addedAndUpdatedFiles) == 0 && len(deletedFiles) == 0 {
			return nil, nil, errors.New("No new files to commit")
		} else {
			// Create a blob for each file
			blobs := []*github.Blob{}
			filePaths := []string{}

			// Delete the files
			// Get the files that are deleted
			fmt.Println("--- Deleted files--")
			fmt.Println(deletedFiles)

			for _, file := range deletedFiles {
				if strings.HasPrefix(file, deletePath) {
					blobs = append(blobs, &github.Blob{
						SHA: nil,
					})

					filePaths = append(filePaths, file)
				}
			}

			// Get the updated files
			fmt.Println("--- Updated files--")
			fmt.Println(addedAndUpdatedFiles)

			for _, file := range addedAndUpdatedFiles {
				blob, _, err := createBlobForFile(ctx, client, repo, file)

				blobs = append(blobs, blob)

				if err != nil {
					return nil, nil, err
				}

				filePaths = append(filePaths, file)
			}

			tree, _, err := createNewTree(ctx, client, repo, blobs, filePaths, currentCommit.GetSHA())
			if err != nil {
				return nil, nil, err
			}

			commit, _, err := createNewCommit(ctx, client, repo, tree, currentCommit, message)
			if err != nil {
				return nil, nil, err
			}

			return setBranchToCommit(ctx, client, repo, branch, commit)
		}
	}
}
