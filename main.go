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
	"strconv"
	"sync"
)

const ConfigFile = "config.json"

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
	Port int `json:"port"`
	LogPath string `json:"logPath"`
	Repos  []SingleConf `json:"repos"`
}

type WebhookResponse struct{
	State string `json:"state"`
	Description string `json:"description"`
	Context string `json:"context"`
	TargetUrl string `json:"target_url"`
}

var configuration AppConfiguration

func copyAndCapture(w io.Writer, r io.Reader) ([]byte, error) {
	var out []byte
	buf := make([]byte, 1024, 1024)
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			d := buf[:n]
			out = append(out, d...)
			_, err := w.Write(d)
			if err != nil {
				return out, err
			}
		}
		if err != nil {
			// Read returns io.EOF at the end of file, which is not an error for us
			if err == io.EOF {
				err = nil
			}
			return out, err
		}
	}
}

func Find(slice []string, val string) (int, bool) {
	for i, item := range slice {
		if item == val {
			return i, true
		}
	}
	return -1, false
}

func execCommandScriptBash(scriptPath string) (string, string){
	cmd := exec.Command(scriptPath)
	var stdout, stderr []byte
	var errStdout, errStderr error
	stdoutIn, _ := cmd.StdoutPipe()
	stderrIn, _ := cmd.StderrPipe()
	err := cmd.Start()
	if err != nil {
		log.Fatalf("cmd.Start() failed with '%s'\n", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		stdout, errStdout = copyAndCapture(os.Stdout, stdoutIn)
		wg.Done()
	}()

	stderr, errStderr = copyAndCapture(os.Stderr, stderrIn)

	wg.Wait()

	err = cmd.Wait()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
	if errStdout != nil || errStderr != nil {
		log.Fatal("failed to capture stdout or stderr\n")
	}
	outStr, errStr := string(stdout), string(stderr)
	log.Printf("STDOUT : %s",outStr)
	log.Printf("STERR : %s",errStr)
	return outStr, errStr
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
	if _, err := os.Stat(ConfigFile); os.IsNotExist(err) {
		log.Panic("%s file does not exist.", ConfigFile)
	}
	f, err := os.OpenFile(configuration.LogPath+"main.log", os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
	content, err := ioutil.ReadFile(ConfigFile)
	if err != nil{
		log.Panic(err)
	}
	_ = json.Unmarshal(content, &configuration)
	r := mux.NewRouter()
	r.HandleFunc("/deploy/{token}", post).Methods(http.MethodPost)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(configuration.Port), r))
}