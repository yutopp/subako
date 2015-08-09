FROM ubuntu:14.04.2
MAINTAINER yutopp

# set system locales to en_US.UTF-8
RUN locale-gen en_US.UTF-8
ENV LANG en_US.UTF-8
ENV LANGUAGE en_US:en
ENV LC_ALL en_US.UTF-8

# basic
RUN apt-get update -qq && apt-get install -qqy make gcc ruby-dev git subversion wget autoconf unzip python g++ curl xz-utils

#
RUN gem install fpm

# addition
RUN apt-get install -qqy mercurial
# makeinfo will be installed by texinfo
RUN apt-get install -qqy automake guile-2.0 guile-2.0-dev autogen gperf gettext flex texinfo m4

#
WORKDIR "/root"