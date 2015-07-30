# Subako
Package building system for Torigoya.

# Requirement
- golang >= 1.3
- docker >= 1.2

# Setup
First, you must create `config.yml` on `app` directory.  
See `config.yml.template`

## Ubuntu(14.04)
### Preparation
#### Docker
If the user runs Subako is `subako`  
Add the user `subako` to `docker` group (Ex. `sudo usermod -aG docker subako`) [see docker reference](https://docs.docker.com/installation/ubuntulinux/)

#### Golang
Download and install golang >= 1.3 from [official golang website](https://golang.org/dl/)

### Build
```
cd image
./build
cd ../
cd app
./build
```
Then, run `./app` to host Subako.
