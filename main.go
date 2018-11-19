package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/joeshaw/envdecode"
	"github.com/minio/minio-go"
)

type Config struct {
	ListenAddr        string `env:"LISTEN_ADDR,default=:9001"`
	S3Endpoint        string `env:"S3_ENDPOINT,default=blockbook-dev.corp:9101"`
	S3AccessKeyID     string `env:"S3_ACCESS_KEY_ID"`
	S3SecretAccessKey string `env:"S3_SECRET_ACCESS_KEY"`
	S3UseSSL          bool   `env:"S3_USE_SSL,default=true"`
}

var (
	config      *Config
	minioClient *minio.Client
)

func init() {
	var c Config
	err := envdecode.Decode(&c)
	if err != nil {
		log.Fatalf("Config parsing failed: %s", err)
	}
	config = &c

	cli, err := minio.New(config.S3Endpoint, config.S3AccessKeyID, config.S3SecretAccessKey, config.S3UseSSL)
	if err != nil {
		log.Fatalf("Connecting to S3 failed: %s", err)
	}
	minioClient = cli
}

func main() {
	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/api/{bucket:[^/]+}/{path:.*}/create", createHandler).Methods("POST")
	router.HandleFunc("/api/{bucket:[^/]+}/{path:.*}/delete", deleteHandler).Methods("DELETE")
	router.HandleFunc("/api/{bucket:[^/]+}/{path:.*}/read", readHandler).Methods("GET")
	router.HandleFunc("/api/{bucket:[^/]+}/{path:.*}/modify", modifyHandler).Methods("PUT")
	router.HandleFunc("/api/{bucket:[^/]+}/{path:.*}/list", listHandler).Methods("GET")

	http.ListenAndServe(config.ListenAddr, router)
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bucket := vars["bucket"]
	path := vars["path"]

	user, email := getUserInfo(r)
	log.Printf("Request: %s %s {user: %s, email: %s, bucket: %s, path: %s}", r.Method, r.URL, user, email, bucket, path)

	defer r.Body.Close()
	rd := bufio.NewReader(r.Body)

	var (
		opts minio.PutObjectOptions
		size int64 = -1
	)

	if t := r.Header.Get("Content-Type"); t != "" {
		opts.ContentType = t
	}
	if s := r.Header.Get("Content-Length"); s != "" {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			size = i
		} else {
			log.Printf("Error parsing Content-Length: %s/%s: %s", bucket, path, err)
		}
	}

	n, err := minioClient.PutObject(bucket, path, rd, size, opts)
	if err != nil {
		log.Printf("Error putting object: %s/%s: %s", bucket, path, verboseError(err))
		http.Error(w, "Error putting object", http.StatusInternalServerError)
		return
	}
	if size != -1 && n != size {
		log.Printf("Error putting object: %s/%s: partial write [%d<%d]", bucket, path, n, size)
		http.Error(w, "Error putting object", http.StatusInternalServerError)
		return
	} else {
		log.Printf("Create %s/%s: successfully uploaded %d bytes", bucket, path, n)
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "%s/%s: OK", bucket, path)
	return
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	// vars := mux.Vars(r)
	// bucket := vars["bucket"]
	// path := vars["path"]
	//
	// user, email := getUserInfo(r)
}

func readHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bucket := vars["bucket"]
	path := vars["path"]

	user, email := getUserInfo(r)
	log.Printf("Request: %s %s {user: %s, email: %s, bucket: %s, path: %s}", r.Method, r.URL, user, email, bucket, path)

	obj, err := minioClient.GetObject(bucket, path, minio.GetObjectOptions{})
	if err != nil {
		log.Printf("Error getting object: %s/%s: %s", bucket, path, verboseError(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer obj.Close()

	h := w.Header()
	if info, err := obj.Stat(); err != nil {
		log.Printf("Error getting object info: %s/%s: %s", bucket, path, verboseError(err))
		h.Set("Content-Type", "application/octet-stream")
	} else {
		h.Set("Content-Type", info.ContentType)
		for k, v := range info.Metadata {
			h[k] = v
		}
	}

	_, err = io.Copy(w, obj)
	if err != nil {
		log.Printf("Error writting response: %s/%s: %s", bucket, path, err)
	}
}

func modifyHandler(w http.ResponseWriter, r *http.Request) {
	// vars := mux.Vars(r)
	// bucket := vars["bucket"]
	// path := vars["path"]
	//
	// user, email := getUserInfo(r)
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bucket := vars["bucket"]
	path := vars["path"]

	user, email := getUserInfo(r)
	log.Printf("Request: %s %s {user: %s, email: %s, path: %s}", r.Method, r.URL, user, email, path)

	doneCh := make(chan struct{})
	defer close(doneCh)

	var objects []string
	for obj := range minioClient.ListObjects(bucket, path, true, doneCh) {
		if obj.Err != nil {
			log.Printf("Error getting list of objects: %s/%s: %s", bucket, path, verboseError(obj.Err))
			http.Error(w, obj.Err.Error(), http.StatusInternalServerError)
			return
		}
		objects = append(objects, obj.Key)
	}

	w.Header().Set("Content-Type", "application/json")
	e := json.NewEncoder(w)
	err := e.Encode(objects)
	if err != nil {
		log.Printf("Error writting resposne: %s/%s: %s", bucket, path, err)
	}
}

func getUserInfo(r *http.Request) (user, email string) {
	user = r.Header.Get("X-User")
	email = r.Header.Get("X-Email")
	return
}

func verboseError(err error) string {
	if err, ok := err.(minio.ErrorResponse); ok {
		return fmt.Sprintf("%s [code: %s, bucket: %s, key: %s, http_status: %d]",
			err.Error(), err.Code, err.BucketName, err.Key, err.StatusCode)
	} else {
		return err.Error()
	}
}
