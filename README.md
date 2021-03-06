# URL Shortener

[![Go Report Card](https://goreportcard.com/badge/github.com/epic-programmer-guy/url-shortener)](https://goreportcard.com/report/github.com/epic-programmer-guy/url-shortener)

This is a simple URL shortener using the [Gin](https://github.com/gin-gonic/gin) framework and the [GORM](https://gorm.io/) ORM.
The only way to add links is to use an api endpoint, meaning that this is not suitable for public websites etc.
The addresses are randomly generated.

To use this project you must add a configuration file as such to the folder in which the binary lies.

## config.json
```
{
    "prefix": "placeholder/",
    "db": "test.db",
    "password": "placeholder"
}
```

You can choose to leave the prefix empty, however choosing a password and a filename for the sqlite database is required.

Additionally you can add an HTML file called badrequest.html to the subfolder ```templates```, which will be displayed when an invalid link beginning with the specified prefix is opened by a user.
Resources for this website, such as stylesheets or images may be placed in the ```resources``` directory, which is statically routed.

## Usage
### Adding Links
The API endpoint to add links is ```127.0.0.1:8080/api/add```
Add new links by POSTing an html form containing the link as ```address``` and the password as ```password```.
A JSON containing the shortened address will be returned to you, unless an error has occured.
I recommend [Postman](https://www.postman.com/) for ease of use.

### Example
![alt Postman screenshot showing how to add links](https://i.imgur.com/CyBBcCo.png)

### Deleting Links
The API endpoint to remove links is ```127.0.0.1:8080/api/remove```

Simply provide the target of a shortened link to remove it.

### Updating Links
Links can be updated by specifying their current target as ```old_address``` and their new target as ```new_address```

The API endpoint to do this is ```127.0.0.1:8080/api/update```