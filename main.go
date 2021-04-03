package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
)

type WebhookRequest struct {
	CallbackURL string `json:"callback_url"`
	PushData    struct {
		Images   []string `json:"images"`
		PushedAt float64      `json:"pushed_at"`
		Pusher   string   `json:"pusher"`
		Tag      string   `json:"tag"`
	} `json:"push_data"`
	Repository struct {
		CommentCount    int    `json:"comment_count"`
		DateCreated     float64    `json:"date_created"`
		Description     string `json:"description"`
		Dockerfile      string `json:"dockerfile"`
		FullDescription string `json:"full_description"`
		IsOfficial      bool   `json:"is_official"`
		IsPrivate       bool   `json:"is_private"`
		IsTrusted       bool   `json:"is_trusted"`
		Name            string `json:"name"`
		Namespace       string `json:"namespace"`
		Owner           string `json:"owner"`
		RepoName        string `json:"repo_name"`
		RepoURL         string `json:"repo_url"`
		StarCount       int    `json:"star_count"`
		Status          string `json:"status"`
	} `json:"repository"`
}

type SingleConf struct{
	Name string `json:"name"`
	Token string `json:"token"`
	Script string `json:"script"`
	Tags []string `json:"tags"`
}

type AppConfiguration struct{
	Repos  []SingleConf `json:"repos"`
}

type WebhookResponse struct{
	State string `json:"state"`
	Description string `json:"description"`
	Context string `json:"context"`
	TargetUrl string `json:"target_url"`
}

var configuration AppConfiguration

func Find(slice []string, val string) (int, bool) {
	for i, item := range slice {
		if item == val {
			return i, true
		}
	}
	return -1, false
}

func execCommandScriptBash(scriptPath string) (output string, exitCode int){
	cmd, err := exec.Command("/bin/sh", scriptPath).Output()
	log.Printf("Executing %s", scriptPath)
	log.Printf("Result : %s", string(cmd))
	exitStatus := 0
	if err != nil{
		exitStatus = 1
	}
	return string(cmd), exitStatus
}

func sendCallback(callbackUrl string, responseToSend WebhookResponse){
	jsonStr, err := json.Marshal(responseToSend)
	if err != nil{
		log.Fatal("Cannot send webhook response")
		return
	}
	req, err := http.NewRequest("POST", callbackUrl, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	log.Printf("POST to %s succeed.", callbackUrl)
	defer resp.Body.Close()
}

func post(w http.ResponseWriter, r *http.Request) {
	var requestWebhook WebhookRequest
	err := json.NewDecoder(r.Body).Decode(&requestWebhook)
	if err != nil{
		log.Fatal("Cannot marshal json request")
	}
	w.Header().Set("Content-Type", "application/json")
	pathParams := mux.Vars(r)
	if val, ok := pathParams["token"]; ok {
		for _, opt := range configuration.Repos{
			if val == opt.Token{
				_, isTagInside := Find(opt.Tags, requestWebhook.PushData.Tag)
				if !isTagInside{
					log.Printf("Repo deploy %s has been detected, but tag %s is not managed.", opt.Name, requestWebhook.PushData.Tag)
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte{})
					return
				}
				log.Printf("Found token. Deploy for %s", opt.Name)
				go execCommandScriptBash(opt.Script)
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte{})
				resStatus := WebhookResponse{
					State:       "success",
					Description: "",
					Context:     "Deploy",
					TargetUrl:   "",
				}
				callUrl := fmt.Sprintf("%v", requestWebhook.CallbackURL)
				go sendCallback(callUrl, resStatus)
				return
			}
		}
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte{})
}


func main() {
	f, err := os.OpenFile("main.log", os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
	content, err := ioutil.ReadFile("config.json")
	if err != nil{
		log.Panic(err)
	}
	_ = json.Unmarshal(content, &configuration)
	r := mux.NewRouter()
	r.HandleFunc("/deploy/{token}", post).Methods(http.MethodPost)
	log.Fatal(http.ListenAndServe(":9001", r))
}