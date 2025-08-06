package main

import (
        "database/sql"
        "encoding/json"
        "fmt"
        "io"
        "log"
        "net/http"
        "os"
        "path/filepath"
        "strconv"
        "strings"

        _ "github.com/go-sql-driver/mysql"
        "github.com/dgrijalva/jwt-go"
        "github.com/aws/aws-sdk-go/aws"
        "github.com/aws/aws-sdk-go/aws/credentials"
        "github.com/aws/aws-sdk-go/aws/session"
        "github.com/aws/aws-sdk-go/service/s3"
)

type Product struct {
        ID       int     `json:"id"`
        Name     string  `json:"name"`
        Price    float64 `json:"price"`
        ImageURL string  `json:"image_url"`
}

type User struct {
        Username string `json:"username"`
        Password string `json:"password"`
}

type Claims struct {
        Username string `json:"username"`
        jwt.StandardClaims
}

var jwtKey = []byte("secret_key")
var s3Session *s3.S3
var bucketName = "healthapp-product-images"

func main() {
        sess, err := session.NewSession(&aws.Config{
                Region: aws.String("ap-southeast-2"),
                Credentials: credentials.NewStaticCredentials(
                        "AKIAWZ5IWGE2QTWN3JU4",
                        "YxLBnEFVPhkvvuXpRyBhRw6UTjrB1uSZxYZWdfMs",
                        ""),
        })
        if err != nil {
                log.Fatalf("Failed to connect to AWS: %v", err)
        }
        s3Session = s3.New(sess)

        http.HandleFunc("/api/login", loginHandler)
        http.HandleFunc("/api/products", productsHandler)
        http.HandleFunc("/api/products/", productHandler)
        http.HandleFunc("/api/upload", uploadHandler)

        port := "8080"
        log.Printf("Backend berjalan di port %s", port)
        log.Fatal(http.ListenAndServe("0.0.0.0:"+port, nil))
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
                http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
                return
        }

        var user User
        if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
                log.Printf("Login Body Parse Error: %v", err)
                http.Error(w, "Invalid Request Body", http.StatusBadRequest)
                return
        }

        db := getDB()
        defer db.Close()

        var storedPassword string
        err := db.QueryRow("SELECT password FROM users WHERE username = ?", user.Username).Scan(&storedPassword)
        if err != nil {
                log.Printf("User not found: %v", err)
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
        }

        if storedPassword != user.Password {
                log.Printf("Invalid password for user: %s", user.Username)
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
        }

        token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{Username: user.Username})
        tokenString, err := token.SignedString(jwtKey)
        if err != nil {
                log.Printf("Token Sign Error: %v", err)
                http.Error(w, "Internal Server Error", http.StatusInternalServerError)
                return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
}

func productsHandler(w http.ResponseWriter, r *http.Request) {
        db := getDB()
        defer db.Close()

        switch r.Method {
        case http.MethodGet:
                rows, err := db.Query("SELECT id, name, price, image_url FROM products")
                if err != nil {
                        log.Printf("DB Query Error: %v", err)
                        http.Error(w, "Database Error", http.StatusInternalServerError)
                        return
                }
                defer rows.Close()

                var products []Product
                for rows.Next() {
                        var p Product
                        if err := rows.Scan(&p.ID, &p.Name, &p.Price, &p.ImageURL); err != nil {
                                log.Printf("Row Scan Error: %v", err)
                                http.Error(w, "Database Error", http.StatusInternalServerError)
                                return
                        }
                        products = append(products, p)
                }

                w.Header().Set("Content-Type", "application/json")
                json.NewEncoder(w).Encode(products)

        case http.MethodPost:
                if !isAuthorized(r) {
                        http.Error(w, "Unauthorized", http.StatusUnauthorized)
                        return
                }

                var payload map[string]interface{}
                if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
                        log.Printf("Product Body Parse Error: %v", err)
                        http.Error(w, "Invalid JSON Body", http.StatusBadRequest)
                        return
                }

                p := Product{}
                if name, ok := payload["name"].(string); ok {
                        p.Name = name
                }
                if priceVal, ok := payload["price"].(float64); ok {
                        p.Price = priceVal
                } else if priceStr, ok := payload["price"].(string); ok {
                        p.Price, _ = strconv.ParseFloat(strings.ReplaceAll(priceStr, ",", ""), 64)
                }
                if imageURL, ok := payload["image_url"].(string); ok {
                        p.ImageURL = imageURL
                }

                result, err := db.Exec("INSERT INTO products (name, price, image_url) VALUES (?, ?, ?)", p.Name, p.Price, p.ImageURL)
                if err != nil {
                        log.Printf("DB Insert Error: %v", err)
                        http.Error(w, "Database Insert Failed", http.StatusInternalServerError)
                        return
                }

                id, _ := result.LastInsertId()
                p.ID = int(id)
                w.Header().Set("Content-Type", "application/json")
                json.NewEncoder(w).Encode(p)

        default:
                http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
        }
}

func productHandler(w http.ResponseWriter, r *http.Request) {
        id := strings.TrimPrefix(r.URL.Path, "/api/products/")
        if id == "" {
                http.Error(w, "ID Required", http.StatusBadRequest)
                return
        }

        db := getDB()
        defer db.Close()

        if !isAuthorized(r) {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
        }

        switch r.Method {
        case http.MethodPut:
                var payload map[string]interface{}
                if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
                        log.Printf("PUT Body Parse Error: %v", err)
                        http.Error(w, "Invalid JSON", http.StatusBadRequest)
                        return
                }

                p := Product{}
                if name, ok := payload["name"].(string); ok {
                        p.Name = name
                }
                if priceVal, ok := payload["price"].(float64); ok {
                        p.Price = priceVal
                } else if priceStr, ok := payload["price"].(string); ok {
                        p.Price, _ = strconv.ParseFloat(strings.ReplaceAll(priceStr, ",", ""), 64)
                }
                if imageURL, ok := payload["image_url"].(string); ok {
                        p.ImageURL = imageURL
                }

                _, err := db.Exec("UPDATE products SET name = ?, price = ?, image_url = ? WHERE id = ?", p.Name, p.Price, p.ImageURL, id)
                if err != nil {
                        log.Printf("DB Update Error: %v", err)
                        http.Error(w, "Update Failed", http.StatusInternalServerError)
                        return
                }
                w.WriteHeader(http.StatusOK)

        case http.MethodDelete:
                _, err := db.Exec("DELETE FROM products WHERE id = ?", id)
                if err != nil {
                        log.Printf("DB Delete Error: %v", err)
                        http.Error(w, "Delete Failed", http.StatusInternalServerError)
                        return
                }
                w.WriteHeader(http.StatusNoContent)

        default:
                http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
        }
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
                http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
                return
        }

        if err := r.ParseMultipartForm(10 << 20); err != nil {
                log.Printf("ParseMultipartForm Error: %v", err)
                http.Error(w, "Invalid Multipart Form", http.StatusBadRequest)
                return
        }

        file, handler, err := r.FormFile("file")
        if err != nil {
                log.Printf("FormFile Error: %v", err)
                http.Error(w, "File Upload Error", http.StatusBadRequest)
                return
        }
        defer file.Close()

        log.Printf("Uploading file: %s", handler.Filename)

        tmpFile, err := os.CreateTemp("", "upload-*")
        if err != nil {
                log.Printf("TempFile Create Error: %v", err)
                http.Error(w, "Temp File Error", http.StatusInternalServerError)
                return
        }
        defer os.Remove(tmpFile.Name())
        defer tmpFile.Close()

        if _, err := io.Copy(tmpFile, file); err != nil {
                log.Printf("File Copy Error: %v", err)
                http.Error(w, "File Save Error", http.StatusInternalServerError)
                return
        }

        tmpFile.Seek(0, 0)
        key := filepath.Base(handler.Filename)

        uploadInput := &s3.PutObjectInput{
                Bucket:      aws.String(bucketName),
                Key:         aws.String(key),
                Body:        tmpFile,
                ContentType: aws.String(handler.Header.Get("Content-Type")),
        }

        _, err = s3Session.PutObject(uploadInput)
        if err != nil {
                log.Printf("S3 Upload Error: %v", err)
                http.Error(w, "S3 Upload Failed", http.StatusInternalServerError)
                return
        }

        imageURL := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, key)
        log.Printf("Upload Success: %s", imageURL)

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"image_url": imageURL})
}

func isAuthorized(r *http.Request) bool {
        authHeader := r.Header.Get("Authorization")
        if authHeader == "" {
                log.Println("Authorization Header Missing")
                return false
        }

        tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
        claims := &Claims{}

        token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
                return jwtKey, nil
        })

        if err != nil || !token.Valid {
                log.Printf("Invalid Token: %v", err)
                return false
        }
        return true
}

func getDB() *sql.DB {
        db, err := sql.Open("mysql", "vorlan:itenas2025@tcp(tgs3db.cdggscqm8xk0.ap-southeast-2.rds.amazonaws.com:3306)/healthapp")
        if err != nil {
                log.Fatalf("DB Connection Error: %v", err)
        }
        return db
}
