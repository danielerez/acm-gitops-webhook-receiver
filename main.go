package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"time"

	"sigs.k8s.io/kustomize/pkg/fs"

	"gopkg.in/go-playground/webhooks.v5/github"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	go_http "gopkg.in/src-d/go-git.v4/plumbing/transport/http"

	"strings"

	"net/http"

	kust "k8s.io/cli-runtime/pkg/kustomize"
)

const (
	hookPath     = "/webhooks"
	confRepoURL  = "https://github.com/danielerez/acm-gitops-kustomize"
	confRepoName = "acm-gitops-kustomize"
	gitHubUser   = "danielerez"
	gitHubToken  = ""
)

func main() {
	hook, _ := github.New()
	http.HandleFunc(hookPath, func(w http.ResponseWriter, r *http.Request) {
		payload, err := hook.Parse(r, github.PushEvent)
		if err != nil {
			if err == github.ErrEventNotFound {
				// ok event wasn't one of the ones asked to be parsed
			}
		}
		switch payload.(type) {

		case github.PushPayload:
			push := payload.(github.PushPayload)

			commit := push.Commits[0]
			if strings.Contains(commit.Message, "variants") {
				log.Println("update variants push - break")
				break
			}
			appFolder := strings.Split(commit.Modified[0], "/")[0]

			GitClone("/tmp/" + confRepoName)
			BuildKustomize("/tmp/" + confRepoName + "/" + appFolder)
			PushVariants("/tmp/"+confRepoName, appFolder)
		}
	})
	http.ListenAndServe(":3000", nil)
}

// GitClone clone the conf repository
func GitClone(path string) {
	log.Println("git clone")

	// clean folder
	os.RemoveAll(path)

	_, err := git.PlainClone(path, false, &git.CloneOptions{
		URL: confRepoURL,
	})
	if err != nil {
		log.Printf("Failed to git clone: %s, %v", path, err)
	}
}

// BuildKustomize execute 'kustomize build'
func BuildKustomize(repoPath string) {
	log.Println("build kustomize")

	// Build a buffer to stream `kustomize build` to
	var buildOutput bytes.Buffer

	// Build using RunKustomizeBuild
	err := kust.RunKustomizeBuild(&buildOutput, fs.MakeRealFS(), repoPath+"/overlays/production")
	if err != nil {
		log.Printf("Failed to kustomize build on: %s, %v", repoPath, err)
	}
	ioutil.WriteFile(repoPath+"/variants/production.yaml", buildOutput.Bytes(), 0644)
}

// PushVariants push new variants commit
func PushVariants(repoPath string, appPath string) {
	// Opens an already existing repository.
	r, _ := git.PlainOpen(repoPath)

	workTree, _ := r.Worktree()

	// Adds the new file to the staging area.
	log.Println(repoPath)
	variantToAdd := appPath + "/variants/production.yaml"
	_, err := workTree.Add(variantToAdd)
	if err != nil {
		log.Printf("Failed to add file: %s, %v", variantToAdd, err)
	}

	workTree.Commit("update variants", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Essential Conf",
			Email: "essential@conf.org",
			When:  time.Now(),
		},
	})

	// push using default options
	err = r.Push(&git.PushOptions{
		Auth: &go_http.BasicAuth{
			Username: gitHubUser,
			Password: gitHubToken,
		},
	})
	if err != nil {
		log.Printf("Failed to push: %v", err)
	}
}
