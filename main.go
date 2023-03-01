package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tidwall/gjson"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type JsonResponse struct {
	AppId           string `json:"AppId"`
	SecretId        string `json:"SecretId"`
	Password        string `json:"Password"`
	PasswordExpired string `json:"PasswordExpired"`
}

type JsonResponseWrapper struct {
	Response map[string]interface{} `json:"response"`
	Message  string                 `json:"message"`
}

func az(args ...string) (stdout string, stderr string, err error) {
	log.SetFlags(0)
	baseCmd := args[0]
	cmdArgs := args[1:]
	cmd := exec.Command(baseCmd, cmdArgs...)
	stdoutbuf, stderrbuf := new(strings.Builder), new(strings.Builder)
	cmd.Stdout = stdoutbuf
	cmd.Stderr = stderrbuf
	err = cmd.Start()
	if err != nil {
		return
	}
	err = cmd.Wait()
	return stdoutbuf.String(), stderrbuf.String(), err
}

func Wrapper(w http.ResponseWriter, r *http.Request) {
	//if r.URL.Path != "/" {
	//	http.Error(w, "404 not found.", http.StatusNotFound)
	//	return
	//}
	switch r.Method {
	case "POST":
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Print("Error reading request body")
			log.Print(err)
		}
		reqBodyBytes := []byte(reqBody)
		var JSON map[string]interface{}
		if err := json.Unmarshal(reqBodyBytes, &JSON); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode("Error unmarshalling request body, please check your payload")
			return
		}

		// modify url path, cut off first and last slash
		var modifiedUrlPath string
		if strings.HasPrefix(r.URL.Path, "/") {
			modifiedUrlPath = r.URL.Path[1:]
		}
		if strings.HasSuffix(r.URL.Path, "/") {
			cutOffLastCharLen := len(r.URL.Path) - 1
			modifiedUrlPath = r.URL.Path[:cutOffLastCharLen]
		}
		azCommand := strings.Split(modifiedUrlPath, "/")

		// add resource group parameters
		if r.URL.Path == "/az/bot/create" || r.URL.Path == "/az/bot/msteams/create" {
			azCommand = append(azCommand, "-g", os.Getenv("AZURE_RESOURCE_GROUP"))
		}

		var azArgs []string
		for key, value := range JSON {
			string := key + "=" + value.(string)
			azArgs = append(azArgs, string)
		}
		azCommand = append(azCommand, azArgs...)
		log.Printf("Run command in console: %v", azCommand)
		stdout, stderr, err := az(azCommand...)

		if err != nil {
			log.Fatalf("error: %s", err)
		}
		//if stderr != "" {
		//	json.NewEncoder(w).Encode(stderr)
		//}
		log.Println(stdout)
		log.Println(stderr)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		var jsonMapStdout map[string]interface{}
		json.Unmarshal([]byte(stdout), &jsonMapStdout)

		//var jsonMapStderr string
		//json.Unmarshal([]byte(stderr), &jsonMapStderr)

		newJson := JsonResponseWrapper{Response: jsonMapStdout, Message: strings.TrimSpace(stderr)}
		json.NewEncoder(w).Encode(newJson)

	default:
		fmt.Fprintf(w, "Sorry, only POST methods are supported.")
	}
}

func CreateBot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		reqBody, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Print("Error reading request body")
			log.Print(err)
		}
		byte := []byte(reqBody)
		var JSON map[string]interface{}
		if err := json.Unmarshal(byte, &JSON); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode("Error unmarshalling request body, please check your payload")
			return
		}
		// get variables from request body
		name := gjson.Get(bytes.NewBuffer(reqBody).String(), "--name").Str
		displayName := gjson.Get(bytes.NewBuffer(reqBody).String(), "--display-name").Str
		endpoint := gjson.Get(bytes.NewBuffer(reqBody).String(), "--endpoint").Str

		// create appRegistration
		stdoutAppRegistration, stderrAppRegistration, err := az("az", "ad", "app", "create", "--display-name", name)
		if err != nil {
			json.NewEncoder(w).Encode(err)
			log.Fatalf("error: %s", err)
		}
		if stderrAppRegistration != "" {
			if !strings.Contains(stderrAppRegistration, "We will patch it") {
				json.NewEncoder(w).Encode(stderrAppRegistration)
				log.Fatalf("stderr: %s", stderrAppRegistration)
			}
		}
		appId := gjson.Get(stdoutAppRegistration, "appId").Str

		// create/reset credential
		stdoutCred, stderrCred, err := az("az", "ad", "app", "credential", "reset", "--id", appId)
		if err != nil {
			json.NewEncoder(w).Encode(err)
			log.Fatalf("error: %s", err)
		}
		if stderrCred != "" {
			if !strings.Contains(stderrCred, "The output includes credentials that you must protect") {
				json.NewEncoder(w).Encode(stderrCred)
				log.Fatalf("stderr: %s", stderrCred)
			}
		}
		password := gjson.Get(stdoutCred, "password").Str

		// create bot
		_, stderrBot, err := az("az", "bot", "create", "--name", name, "--app-type", "MultiTenant", "--display-name", displayName, "--endpoint", endpoint, "--appid", appId, "-g", os.Getenv("AZURE_RESOURCE_GROUP"))
		if err != nil {
			json.NewEncoder(w).Encode(err)
			log.Fatalf("error: %s", err)
		}
		if stderrBot != "" {
			if !strings.Contains(stderrBot, "Provided bot name already exists in Resource Group") {
				json.NewEncoder(w).Encode(stderrBot)
				log.Fatalf("stderr: %s", stderrBot)
			}
		}

		// create msteams channel
		_, stderrChannel, err := az("az", "bot", "msteams", "create", "--name", name, "-g", os.Getenv("AZURE_RESOURCE_GROUP"))
		if err != nil {
			json.NewEncoder(w).Encode(err)
			log.Fatalf("error: %s", err)
		}
		if stderrBot != "" {
			if !strings.Contains(stderrChannel, "is in preview and under development") {
				json.NewEncoder(w).Encode(stderrChannel)
				log.Fatalf("stderr: %s", stderrChannel)
			}
		}
		Response := JsonResponse{AppId: appId, Password: password, SecretId: "TODO", PasswordExpired: "TODO"}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Response)
		log.Printf("Bot created successfully, appId: %s", appId)

	default:
		fmt.Fprintf(w, "Sorry, only POST method are supported.")
	}
}

func azLogin() {
	b := new(strings.Builder)
	s := exec.Command("az", "login", "--service-principal", "-u", os.Getenv("AZURE_LOGIN"), "-p", os.Getenv("AZURE_PASSWORD"), "--tenant", os.Getenv("AZURE_TENANT"))
	s.Stdout = b
	s.Run()
	log.Println(b.String())
}

func main() {
	azLogin()
	http.HandleFunc("/", Wrapper)
	http.HandleFunc("/create-bot", CreateBot)
	fmt.Printf("Starting on 8080 port...\n")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
