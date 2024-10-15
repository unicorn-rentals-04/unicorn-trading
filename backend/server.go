package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql" // sql/database mysql driver
)

func main() {
	startMode := os.Getenv("START_MODE")
	if startMode == "REPORTER" {
		bucketname := os.Getenv("ECOMM_BUCKET")
		bucketregion := os.Getenv("ECOMM_STATICREGION")
		bucketendpoint := os.Getenv("ECOMM_OBJECTSTORAGEENDPOINT")
		StartReporter(bucketendpoint, bucketname, "", "", bucketregion)
	} else {
		reporterEndpoint := os.Getenv("ECOMM_ENDPOINT")
		dbType := "mysql"
		dbUserName := os.Getenv("ECOMM_DATABASEUSER")
		dbPassword := os.Getenv("ECOMM_DATABASEPASS")
		dbHost := os.Getenv("ECOMM_DATABASEHOST")
		dbPort := os.Getenv("ECOMM_DATABASEPORT")
		dbName := os.Getenv("ECOMM_DATABASENAME")
		// username:password@tcp(127.0.0.1:3306)/jazzrecords
		dbConnString := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUserName, dbPassword, dbHost, dbPort, dbName)
		buildRoot := "/tmp/"
		authtoken := os.Getenv("ECOMM_AUTHTOKEN")
		StartFrontend(reporterEndpoint, dbType, dbConnString, buildRoot, authtoken)
	}

}

func simpleTokenAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		suppliedToken := c.GetHeader("X-Auth-Token")
		if suppliedToken == "" || suppliedToken != token {
			c.Abort()
			sendError(c, http.StatusUnauthorized, errors.New("authentication required"))
		}
	}
}

func httpJSONPost(endpoint string, data io.Reader) (*http.Response, []byte, error) {
	req, err := http.NewRequest("POST", endpoint, data)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	return resp, body, nil
}

func httpJSONGet(endpoint string) (*http.Response, []byte, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	return resp, body, nil
}

func getOrders(db *sql.DB) (string, error) {
	rows, err := db.QueryContext(context.Background(), "select * from orders order by id desc limit 5000")
	if err != nil {
		return "", err
	}

	// adapted from https://stackoverflow.com/a/60386531 - database/sql rows to json
	cT, err := rows.ColumnTypes()
	if err != nil {
		return "", err
	}

	count := len(cT)
	marshalFromRows := []interface{}{}

	for rows.Next() {
		scanArgs := make([]interface{}, count)

		for i, v := range cT {
			switch v.DatabaseTypeName() {
			case "VARCHAR", "TEXT", "UUID", "TIMESTAMP":
				scanArgs[i] = new(sql.NullString)
			case "BOOL":
				scanArgs[i] = new(sql.NullBool)
			case "INT4":
				scanArgs[i] = new(sql.NullInt64)
			default:
				scanArgs[i] = new(sql.NullString)
			}
		}

		err := rows.Scan(scanArgs...)
		if err != nil {
			log.Fatalf("failed to copy rows, %s", err.Error())
		}

		convertedRows := map[string]interface{}{}
		for i, v := range cT {
			if z, ok := (scanArgs[i]).(*sql.NullBool); ok {
				convertedRows[v.Name()] = z.Bool
				continue
			}
			if z, ok := (scanArgs[i]).(*sql.NullString); ok {
				convertedRows[v.Name()] = z.String
				continue
			}
			if z, ok := (scanArgs[i]).(*sql.NullInt64); ok {
				convertedRows[v.Name()] = z.Int64
				continue
			}
			if z, ok := (scanArgs[i]).(*sql.NullFloat64); ok {
				convertedRows[v.Name()] = z.Float64
				continue
			}
			if z, ok := (scanArgs[i]).(*sql.NullInt32); ok {
				convertedRows[v.Name()] = z.Int32
				continue
			}
			convertedRows[v.Name()] = scanArgs[i]
		}

		marshalFromRows = append(marshalFromRows, convertedRows)
	}

	z, err := json.Marshal(marshalFromRows)
	if err != nil {
		return "", err
	}

	return string(z), err
}

func sendError(c *gin.Context, code int, err error) {
	c.JSON(code, gin.H{"error": err.Error()})
}

func StartFrontend(reporterEndpoint string, dbType string, dbConnString string, buildRoot string, authtoken string) {
	db, err := sql.Open(dbType, dbConnString)
	if err != nil {
		log.Fatal(err.Error())
	}

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowHeaders = append(corsConfig.AllowHeaders, "x-auth-token", "X-Auth-Token")

	r := gin.Default()
	r.Use(cors.New(corsConfig))
	r.Use(static.Serve("/", static.LocalFile(buildRoot, true)))
	g := r.Group("/api")
	g.GET("/orders", func(c *gin.Context) {
		o, err := getOrders(db)
		if err != nil {
			sendError(c, http.StatusBadRequest, err)
			return
		}
		c.String(http.StatusOK, o)
	})
	g.GET("/archives", func(c *gin.Context) {
		var ret string
		archiveURL := c.Query("archiveUrl")

		// fetch single report if reportURL param
		if archiveURL != "" {
			var fetchURL string
			switch { // enable lookups from all reporting services from this instance
			case strings.HasPrefix(archiveURL, "http"):
				fetchURL = archiveURL
			case strings.HasPrefix(archiveURL, "/archive/"): // support older url lookups
				fetchURL = fmt.Sprintf("%s/api/archives/%s", reporterEndpoint, strings.Split(archiveURL, "/archive/")[1])
			default: // enable lookups for the well-known configured endpoint
				fetchURL = fmt.Sprintf("%s/api/archives/%s", reporterEndpoint, archiveURL)
			}

			resp, body, err := httpJSONGet(fetchURL)
			if err != nil {
				sendError(c, http.StatusInternalServerError, err)
				return
			}
			if resp.StatusCode != 200 {
				sendError(c, http.StatusInternalServerError, errors.New(string(body)))
				return
			}

			// TODO Convert into report type
			ret = string(body)
		} else {
			// fetch archives if no reportURL param
			resp, bits, err := httpJSONGet(fmt.Sprintf("%s/api/archives", reporterEndpoint))
			if err != nil {
				sendError(c, http.StatusInternalServerError, err)
				return
			}
			if resp.StatusCode != 200 {
				sendError(c, http.StatusInternalServerError, errors.New(string(bits)))
				return
			}

			var reports []ArchiveURL
			if err := json.Unmarshal(bits, &reports); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, reports)
			return
		}

		c.String(http.StatusOK, ret)
	})

	type RunCommandData struct {
		UseShell bool     `json:"useShell"`
		Command  []string `json:"command"`
	}

	g.POST("/archives", func(c *gin.Context) {
		// Persist the POST body (json) via the reporter
		resp, bits, err := httpJSONPost(fmt.Sprintf("%s/api/archive", reporterEndpoint), c.Request.Body)
		if err != nil {
			sendError(c, http.StatusInternalServerError, err)
			return
		}
		if resp.StatusCode != 201 {
			sendError(c, http.StatusInternalServerError, errors.New(string(bits)))
			return
		}

		var retdata map[string]interface{}
		if err := json.Unmarshal(bits, &retdata); err != nil {
			sendError(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusCreated, gin.H{"url": retdata["key"]})
	})

	g.POST("/pty", simpleTokenAuth(authtoken), func(ctx *gin.Context) {
		// TODO: authorization
		body, err := ioutil.ReadAll(ctx.Request.Body)
		if err != nil {
			sendError(ctx, http.StatusInternalServerError, err)
			return
		}
		fmt.Println(string(body))

		var v RunCommandData
		if err := json.Unmarshal(body, &v); err != nil {
			sendError(ctx, http.StatusBadRequest, err)
			return
		}

		var cmd *exec.Cmd
		if v.UseShell {
			newcmd := []string{"/bin/bash", "-c", strings.Join(v.Command, " ")}
			cmd = exec.Command(newcmd[0], newcmd[1:]...)
		} else {
			cmd = exec.Command(v.Command[0], v.Command[1:]...)
		}

		fmt.Println(cmd.String())
		b, err := cmd.Output()
		if err != nil {
			e, ok := err.(*exec.ExitError)
			if ok {
				ctx.JSON(http.StatusOK, map[string]string{"response": string(e.Stderr)})
				return
			}
			ctx.JSON(http.StatusOK, map[string]string{"response": string(err.Error())})
			return
		}

		ctx.JSON(http.StatusOK, map[string]string{"response": string(b)})
	})

	r.NoRoute(func(ctx *gin.Context) {
		ctx.Redirect(http.StatusPermanentRedirect, "/")
	})

	if err := r.Run(":8081"); err != nil {
		log.Fatal(err)
	}
}

type ArchiveURL struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func StartReporter(objectStorageEndpoint string, bucketName string, accessKey string, secretAccessKey string, staticRegion string) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal(err.Error())
	}

	if staticRegion != "" {
		cfg.Region = staticRegion
	}

	if accessKey != "" && secretAccessKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretAccessKey, "")
	}

	if objectStorageEndpoint != "" {
		staticResolver := aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
			return aws.Endpoint{
				PartitionID:       "aws",
				URL:               objectStorageEndpoint,
				SigningRegion:     staticRegion,
				HostnameImmutable: true,
			}, nil
		})
		cfg.EndpointResolver = staticResolver
	}

	s3Client := s3.NewFromConfig(cfg)
	r := gin.Default()
	r.Use(cors.Default())
	g := r.Group("/api")
	g.GET("/archives", func(c *gin.Context) {
		keys := []ArchiveURL{}

		out, err := s3Client.ListObjects(context.Background(), &s3.ListObjectsInput{Bucket: aws.String(bucketName)})
		if err != nil {
			fmt.Println(err)
			c.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
			return
		}
		for _, o := range out.Contents {
			keys = append(keys, ArchiveURL{
				Name: *o.Key,
				URL:  *o.Key,
			})
		}

		// Get S3 listing, parse it, and return ids (epoch)
		c.JSON(http.StatusOK, keys)
	})

	g.GET("/archives/:id", func(c *gin.Context) {
		id, ok := c.Params.Get("id")
		if !ok {
			c.JSON(http.StatusInternalServerError, map[string]string{"err": "must supply archive id"})
			return
		}

		obj, err := s3Client.GetObject(context.Background(), &s3.GetObjectInput{Bucket: aws.String(bucketName), Key: aws.String(id)})
		if err != nil {
			fmt.Println(err)
			c.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
			return
		}

		bits, err := ioutil.ReadAll(obj.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
			return
		}

		// Get S3 object for id; where id == epoch
		c.String(http.StatusOK, string(bits))
	})
	g.POST("/archive", func(c *gin.Context) {
		// Persist the POST body (json) to a new s3 object named :id
		objName := fmt.Sprintf("%d", time.Now().Unix())

		uploader := manager.NewUploader(s3Client)
		result, err := uploader.Upload(context.TODO(), &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objName),
			Body:   c.Request.Body,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, map[string]string{"key": *result.Key})
	})

	if err := r.Run(":9999"); err != nil {
		log.Fatal(err)
	}
}
