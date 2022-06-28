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

type LinkUpdateRequest struct {
	OldAddress string `form:"old_address" json:"old_address" xml:"old_address" binding:"required"`
	NewAddress string `form:"new_address" json:"new_address" xml:"new_address" binding:"required"`
	Password   string `form:"password" json:"password" xml:"password" binding:"required"`
}

type Link struct {
	gorm.Model
	Id     uint32
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
	router.POST("/api/add", func(context *gin.Context) {
		addLink(context, db)
	})

	///
	/// API endpoint for removing links
	///
	router.POST("/api/remove", func(context *gin.Context) {
		removeLink(context, db)
	})

	///
	/// API endpoint for updating existing links (for example if a resource has been moved)
	///
	router.POST("/api/update", func(context *gin.Context) {
		updateLink(context, db)
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

	re, err := reformatUrl(requestBody.Address)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{
			"message": "Error parsing URL",
		})
	}

	link := Link{Target: re}
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

func removeLink(context *gin.Context, db *gorm.DB) {
	requestBody := LinkRequest{}
	context.Bind(&requestBody)

	if md5.Sum([]byte(requestBody.Password)) != config.hashed { // Exits if a wrong password was provided
		context.JSON(http.StatusUnauthorized, gin.H{})
		return
	}

	re, err := reformatUrl(requestBody.Address)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{
			"message": "Error parsing URL",
		})
		return
	}

	var link Link
	db.First(&link, "target = ?", re)
	if link.Target == "" {
		context.JSON(http.StatusBadRequest, gin.H{
			"message": "Link not found",
		})
		return
	}
	db.Delete(&link, 1)
	db.Save(&link)

	println("Removed a link pointing to " + requestBody.Address)
	context.JSON(200, gin.H{
		"message": requestBody.Address + " removed",
	})
}

func updateLink(context *gin.Context, db *gorm.DB) {
	requestBody := LinkUpdateRequest{}
	context.Bind(&requestBody)

	if md5.Sum([]byte(requestBody.Password)) != config.hashed { // Exits if a wrong password was provided
		context.JSON(http.StatusUnauthorized, gin.H{})
		return
	}

	oldRe, err := reformatUrl(requestBody.OldAddress)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{
			"message": "Error parsing first URL",
		})
		return
	}

	var link Link
	db.First(&link, "target = ?", oldRe)
	if link.Target == "" {
		context.JSON(http.StatusBadRequest, gin.H{
			"message": "Link not found",
		})
		return
	}

	newRe, err := reformatUrl(requestBody.NewAddress)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{
			"message": "Error parsing second URL",
		})
		return
	}

	db.Model(&link).Update("Target", newRe)

	println("Update a link that pointed to " + requestBody.OldAddress + "\nto point to " + requestBody.NewAddress)
	context.JSON(200, gin.H{
		"message": "Successfully updated Link to now point to " + requestBody.NewAddress,
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

func reformatUrl(s string) (string, error) {
	var u *url.URL
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}

	if u.Scheme == "" { //Default to https if no scheme was provided
		u, err = url.Parse("https://" + u.String()) // The URL object must be created again, so that the host can correctly be identified
		if err != nil {
			return "", err
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
		return "", errors.New("Malformed URL")
	}

	return u.String(), nil
}

func parseIdString(idString string) (uint32, error) {
	for _, element := range idString { //This for-loop reverses the changes made to a standard base32 representation of a given integer
		if element > 'o' {
			element--
		} // Yes o
		if element >= 'l' {
			element--
		} // Yes l
		if element >= 'i' {
			element--
		} // Yes i
		element-- // Yes 0
	}
	i, err := strconv.ParseUint(idString, 32, 32)
	return uint32(i), err
}

func parseIdInt(idInt uint32) string {
	idString := strconv.FormatInt(int64(idInt), 32)
	for _, element := range idString { //This for-loop changes what characters are used within the idString
		element++ // No 0
		if element >= 'i' {
			element++
		} // No i
		if element >= 'l' {
			element++
		} // No l
		if element >= 'o' {
			element++
		} // No o
	}
	return idString
}

func generateUnusedId(db *gorm.DB) uint32 {
	r := Link{Target: "some random text"}
	for r.Target != "" {
		r.Id = rand.Uint32() >> 12
		r.Target = ""                // (Re-)Set target to ""...
		db.First(&r, "id = ?", r.Id) // ...except if the id happens to already be in use
	}
	return r.Id
}
