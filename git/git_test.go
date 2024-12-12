package git

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v67/github"
)

func TestCreateBlobForFile(t *testing.T) {
	// Create a temporary file with some content
	tmpfile, err := os.CreateTemp("", "git-test")

	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.WriteString("Hello, World!")

	if err != nil {
		t.Fatal(err)
	}

	host, _ := auth.DefaultHost()
	token, _ := auth.TokenForHost(host)
	
	client := github.NewClient(nil).WithAuthToken(token)

	if err != nil {
		t.Fatal(err)
	}

	repo := repository.Repository{
		Owner: "firestartr-test",
		Name:  "ts-node-runtime-poc",
	}

	// Create a blob for the file

	ctx := context.Background()
	blob, _, err := createBlobForFile(ctx, client, repo, tmpfile.Name())

	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(*blob.URL)

}
