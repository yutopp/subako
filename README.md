# Subako
Package building system for Torigoya.

# Requirement
- golang >= 1.3
- docker >= 1.2
- reprepro
- npm
- bower

# Setup
First, you must create `config.yml` on `app` directory.  
See `config.yml.template`

## Ubuntu(14.04)
### Preparation
(Please skip depending on your case)
```
sudo apt-get install software-properties-common
sudo apt-get update
sudo apt-get install nodejs npm reprepro
sudo ln -s /usr/bin/node /usr/local/bin/node
```

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
bower update
```
Then, run `./bin/server` to host Subako.
