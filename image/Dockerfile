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
RUN apt-get install -qqy bison

# For jdk
# TODO: divide files
RUN apt-get -qq update
RUN apt-get -qqy --fix-missing install openjdk-7-jdk
RUN apt-get -qqy --fix-missing install libX11-dev libxext-dev libxrender-dev libxtst-dev libxt-dev
RUN apt-get -qqy --fix-missing install libcups2-dev
RUN apt-get -qqy --fix-missing install libfreetype6-dev
RUN apt-get -qqy --fix-missing install libasound2-dev
RUN apt-get -qqy --fix-missing install ccache

RUN apt-get -qqy --fix-missing install zip

# Set the timezone
RUN echo "Asia/Tokyo " > /etc/timezone
RUN dpkg-reconfigure -f noninteractive tzdata

#
WORKDIR "/root"