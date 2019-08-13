FROM ubuntu:19.04
RUN apt-get update && apt-get -y install ca-certificates git apache2 apache2-utils
RUN a2enmod cgi alias env rewrite

RUN mkdir /var/www/repos
RUN mkdir /var/www/secure

COPY 000-default.conf /etc/apache2/sites-enabled/000-default.conf
COPY .htpasswd /var/www/secure/.htpasswd
COPY ports.conf /etc/apache2/ports.conf

RUN chgrp -R www-data /var/www/repos

EXPOSE 9797
CMD ["apachectl", "-D", "FOREGROUND"]