GOPATH=`pwd` \
      go get github.com/fsouza/go-dockerclient \
      github.com/flosch/pongo2 \
      github.com/zenazn/goji \
      github.com/zenazn/goji/web \
      gopkg.in/yaml.v2 \
      github.com/ActiveState/tail \
      github.com/jinzhu/gorm \
      github.com/mattn/go-sqlite3 \
      github.com/robfig/cron \
    || exit -1

echo "building..."
GOPATH=`pwd` go build