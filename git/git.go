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

const (
	exitOk         int = 0
	exitError      int = 1
	exitNoNewFiles int = 10
	emptyTreeSHA       = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
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

// Returns the name of the currently checked out branch (HEAD) in the given
// repository path, to use as the base branch for the commit.
func GetBaseBranch(repoPath string) (string, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", err
	}

	head, err := repo.Head()
	if err != nil {
		return "", err
	}
	baseBranchName := strings.Split(head.Name().String(), "/")[2]

	return baseBranchName, nil
}

func getCurrentCommit(
	ctx context.Context,
	client *github.Client,
	repo repository.Repository,
	branch string,
) (*github.RepositoryCommit, error) {
	// Get the branch reference
	branchRef, _, err := client.Git.GetRef(
		ctx, repo.Owner, repo.Name, fmt.Sprintf("heads/%s", branch),
	)

	if err != nil {
		return nil, err
	}

	// Get the commit SHA
	commitSha := branchRef.Object.SHA

	// Get the commit
	commit, _, err := client.Repositories.GetCommit(
		ctx, repo.Owner, repo.Name, *commitSha, nil,
	)

	if err != nil {
		return nil, err
	}

	return commit, nil
}

func createBlobForFile(
	ctx context.Context,
	client *github.Client,
	repo repository.Repository,
	file string,
) (*github.Blob, *github.Response, error) {
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

func createNewTree(
	ctx context.Context,
	client *github.Client,
	repo repository.Repository,
	blobs []*github.Blob,
	blobPaths []string,
	parentTreeSha string,
) (*github.Tree, *github.Response, error) {
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

func createNewCommit(
	ctx context.Context,
	client *github.Client,
	repo repository.Repository,
	currentTree *github.Tree,
	parentCommit *github.RepositoryCommit,
	message string,
) (*github.Commit, *github.Response, error) {
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

func createNewEmtpyTreeCommit(
	ctx context.Context,
	client *github.Client,
	repo repository.Repository,
	parentCommit *github.RepositoryCommit,
	message string,
) (*github.Commit, *github.Response, error) {
	commit := &github.Commit{
		Message: github.String(message),
		Tree:    &github.Tree{SHA: github.String(emptyTreeSHA)},
		Parents: []*github.Commit{
			{
				SHA: github.String(parentCommit.GetSHA()),
			},
		},
	}

	return client.Git.CreateCommit(ctx, repo.Owner, repo.Name, commit, nil)
}

func setBranchToCommit(
	ctx context.Context,
	client *github.Client,
	repo repository.Repository,
	branch string,
	commit *github.Commit,
) (*github.Reference, *github.Response, error) {

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

func getGroupedFiles(
	fileStatuses git.Status,
) ([]string, []string, []string, error) {
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
			return nil, nil, nil, fmt.Errorf(
				"Unsupported status code %c for file %s",
				status.Worktree,
				file,
			)
		}
	}

	return addedFiles, updatedFiles, deletedFiles, nil
}

func checkIfAllFilesDeleted() (bool, error) {
	filenameList, err := os.ReadDir(".")
	if err != nil {
		return false, err
	}

	if len(filenameList) == 1 && filenameList[0].Name() == ".git" {
		return true, nil
	}

	return false, nil
}

func UploadToRepo(
	ctx context.Context,
	client *github.Client,
	repo repository.Repository,
	path string,
	deletePath string,
	branch string,
	baseBranch string,
	message string,
	createEmptyCommit *bool,
	allowEmptyCommit *bool,
	allowEmptyTree *bool,
) (*github.Reference, *github.Response, error, int) {
	// Get the current currentCommit
	currentCommit, err := getCurrentCommit(ctx, client, repo, baseBranch)
	if err != nil {
		return nil, nil, err, exitError
	}

	allFilesDeleted, err := checkIfAllFilesDeleted()
	if err != nil {
		return nil, nil, err, exitError
	}

	if allFilesDeleted {
		if *allowEmptyTree {
			fmt.Println("All files from the repository have been deleted.")
			fmt.Println("--allow-empty-tree flag is set.")
			fmt.Println("Committing an empty tree to the branch...")
			emptyTreeCommit, _, err := createNewEmtpyTreeCommit(
				ctx, client, repo, currentCommit, message,
			)
			if err != nil {
				return nil, nil, err, exitError
			}

			ref, resp, respErr := setBranchToCommit(
				ctx, client, repo, branch, emptyTreeCommit,
			)
			if respErr != nil {
				return nil, nil, respErr, exitError
			}

			return ref, resp, respErr, exitOk
		}

		// If all files are deleted and allowEmptyTree is not set, return an error
		return nil, nil, errors.New(
			"All files in the repository have been deleted, but the " +
				"--allow-empty-tree parameter has not been set to true. " +
				"Please use it if you actually want to commit these changes " +
				"(the repo will be empty as the result). Aborting.",
		), exitError
	}

	fileStatuses, err := getGitPorcelain(path)

	addedFiles, updatedFiles, deletedFiles, err := getGroupedFiles(fileStatuses)
	if err != nil {
		return nil, nil, err, exitError
	}
	addedAndUpdatedFiles := append(updatedFiles, addedFiles...)

	if (len(addedAndUpdatedFiles) == 0 &&
		len(deletedFiles) == 0 &&
		*allowEmptyCommit) || *createEmptyCommit {
		// In order to push an empty commit, we first need to create a
		// dummy file and commit it to the branch
		fileName := fmt.Sprintf("%x", sha256.Sum256(
			[]byte("firestartr-empty-commit-dummy.txt"),
		))
		dummy, err := os.Create(fileName)
		if err != nil {
			return nil, nil, err, exitError
		}
		defer dummy.Close()

		blob, _, err := createBlobForFile(ctx, client, repo, fileName)
		if err != nil {
			return nil, nil, err, exitError
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
			return nil, nil, err, exitError
		}

		commit, _, err := createNewCommit(
			ctx, client, repo, tree,
			currentCommit, message,
		)
		if err != nil {
			return nil, nil, err, exitError
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
			return nil, nil, err, exitError
		}

		emptyCommit, _, err := createNewCommit(
			ctx, client, repo, emptyTree,
			currentCommit, message,
		)
		if err != nil {
			return nil, nil, err, exitError
		}

		ref, resp, respErr := setBranchToCommit(ctx, client, repo, branch, emptyCommit)
		if respErr != nil {
			return nil, nil, respErr, exitError
		}

		err = os.Remove(fileName)
		if err != nil {
			return nil, nil, err, exitError
		}

		return ref, resp, respErr, exitOk
	} else {
		if len(addedAndUpdatedFiles) > 0 || len(deletedFiles) > 0 {
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
					return nil, nil, err, exitError
				}

				filePaths = append(filePaths, file)
			}

			tree, _, err := createNewTree(ctx, client, repo, blobs, filePaths, currentCommit.GetSHA())
			if err != nil {
				return nil, nil, err, exitError
			}

			commit, _, err := createNewCommit(ctx, client, repo, tree, currentCommit, message)
			if err != nil {
				return nil, nil, err, exitError
			}

			ref, resp, err := setBranchToCommit(ctx, client, repo, branch, commit)
			if err != nil {
				return nil, nil, err, exitError
			}

			return ref, resp, err, exitOk
		} else {
			return nil, nil, errors.New("no new files to commit"), exitNoNewFiles
		}
	}
}
