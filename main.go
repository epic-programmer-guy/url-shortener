package main

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type LinkRequest struct {
	Address  string `form:"address" json:"address" xml:"address" binding:"required"`
	Password string `form:"password" json:"password" xml:"password" binding:"required"`
}

type Link struct {
	gorm.Model
	Id     uint16
	Target string
}

type Config struct {
	Prefix   string `json:"prefix"`
	DBname   string `json:"db"`
	Password string `json:"password"`
	hashed   [16]byte
}

var config Config

func main() {
	file, err := ioutil.ReadFile("config.json")

	if err != nil {
		log.Fatalln("config.json not found")
	}

	json.Unmarshal(file, &config) // Initialize the configuration from the config.json file

	println("URL shortener address prefix: /" + config.Prefix)

	if config.Password == "" {
		log.Fatalln("no password specified in config.json")
	}
	println("password: " + config.Password[0:1] + "*********")
	config.hashed = md5.Sum([]byte(config.Password))

	if config.DBname == "" {
		log.Fatalln("no database file specified in config.json")
	}
	println("using database " + config.DBname)

	db, err := gorm.Open(sqlite.Open(config.DBname), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})

	if err != nil {
		log.Fatalln("failed to connect database")
	}
	db.AutoMigrate(&Link{})

	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()
	router.SetTrustedProxies(nil) // If you're using a CDN or some other kind of proxy this does not work.

	//
	// Loads all HTML templates within the templates folder
	//
	router.LoadHTMLGlob("templates/*") //LoadHTMLGlob enables loading a batch of templates at once

	///
	/// API endpoint for adding the links
	///
	router.POST("/"+config.Prefix+"add", func(context *gin.Context) {
		addLink(context, db)
	})

	///
	/// Redirect
	///
	router.GET("/"+config.Prefix+":id", func(context *gin.Context) {
		redirectToTarget(context, db)
	})

	///
	/// /prefix/
	///
	router.GET("/"+config.Prefix, func(context *gin.Context) {
		context.HTML(http.StatusBadRequest, "badrequest.html", gin.H{"title": "400 - Bad Request"})
	})

	///
	///	static route for all resources within the "resources" folder
	///
	router.Static("/resources", "./resources")

	router.Run(":8080")
}

func addLink(context *gin.Context, db *gorm.DB) {
	requestBody := LinkRequest{}
	context.Bind(&requestBody)

	if md5.Sum([]byte(requestBody.Password)) != config.hashed { // Exits if a wrong password was provided
		context.JSON(http.StatusUnauthorized, gin.H{})
		return
	}

	println("Reformatting URL...")
	var u *url.URL
	u, err := url.Parse(requestBody.Address)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{
			"message": "Couldn't parse provided URL",
		})
		return
	}

	if u.Scheme == "" { //Default to https if no scheme was provided
		u, err = url.Parse("https://" + u.String()) // The URL object must be created again, so that the host can correctly be identified
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{
				"message": "Couldn't parse provided URL",
			})
			return
		}
	}
	if strings.Split(u.Host, ".")[0] == "www" { // If the adress starts with "www."...
		u.Host = strings.Replace(u.Host, "www.", "", 1) // ...replace it with ""
	}
	var splits uint8
	splits = 1
	flag := false
	for _, element := range strings.Split(u.Host, ".") {
		splits++
		if len(element) < 1 {
			flag = true
			break
		}
	}

	if flag || splits < 2 {
		context.JSON(http.StatusBadRequest, gin.H{
			"message": "Malformed URL",
		})
		return
	}

	requestBody.Address = u.String()

	link := Link{Target: requestBody.Address}
	result := db.First(&link, "target = ?", link.Target)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) { //First or create sadly does not allow you to override id
		println("Adding new link...")
		link.Id = generateUnusedId(db)
		db.Create(&link)
	}
	db.Save(&link)

	context.JSON(200, gin.H{
		"address": "/" + config.Prefix + parseIdInt(link.Id),
	})
}

func redirectToTarget(context *gin.Context, db *gorm.DB) {
	var link Link
	id, err := parseIdString(context.Param("id"))

	if err != nil {
		context.HTML(http.StatusBadRequest, "badrequest.html", gin.H{"title": "400 - Bad Request"})
		return
	}

	db.First(&link, "id = ?", id)

	if link.Target == "" {
		context.HTML(http.StatusBadRequest, "badrequest.html", gin.H{"title": "400 - Bad Request"})
		return
	}

	context.Redirect(http.StatusMovedPermanently, link.Target)
}

func parseIdString(idString string) (uint16, error) {
	i, err := strconv.ParseUint(idString, 16, 32)
	return uint16(i), err
}

func parseIdInt(idInt uint16) string {
	return strconv.FormatUint(uint64(idInt), 16)
}

func generateUnusedId(db *gorm.DB) uint16 {
	r := Link{Target: "some random text"}
	for r.Target != "" {
		r.Id = uint16(rand.Uint32())
		r.Target = ""                // (Re-)Set target to ""...
		db.First(&r, "id = ?", r.Id) // ...except if the id happens to already be in use
	}
	return r.Id
}
