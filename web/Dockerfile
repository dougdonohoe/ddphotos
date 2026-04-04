FROM httpd:2.4

# Point DocumentRoot at build/ inside the mounted web/ directory.
# Mounting web/ (not web/build/) means npm rebuilds, which delete and
# recreate build/, don't break the bind mount's inode binding.
RUN sed -i 's|DocumentRoot "/usr/local/apache2/htdocs"|DocumentRoot "/usr/local/apache2/htdocs/build"|g' /usr/local/apache2/conf/httpd.conf \
    && sed -i 's|<Directory "/usr/local/apache2/htdocs">|<Directory "/usr/local/apache2/htdocs/build">|g' /usr/local/apache2/conf/httpd.conf \
    && sed -i 's/#LoadModule rewrite_module/LoadModule rewrite_module/' /usr/local/apache2/conf/httpd.conf \
    && sed -i 's/AllowOverride None/AllowOverride All/g' /usr/local/apache2/conf/httpd.conf \
    && echo "ServerName localhost" >> /usr/local/apache2/conf/httpd.conf \
    && printf '\n<Directory "/usr/local/apache2/htdocs/build">\n    Options +FollowSymLinks\n</Directory>\n' >> /usr/local/apache2/conf/httpd.conf

# entrypoint.sh symlinks album data from /albums into build/albums/ at startup,
# then hands off to httpd-foreground.
COPY entrypoint.sh /entrypoint.sh
CMD ["/entrypoint.sh"]

# web/ dir is mounted at runtime via -v; album data at /albums; nothing to COPY here.
# docker run --rm -p 8080:80 -v $(PWD)/web:/usr/local/apache2/htdocs -v $ALBUMS_DIR/$SITE_ID:/albums:ro photos-apache
