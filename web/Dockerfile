FROM httpd:2.4

# Point DocumentRoot at build/ inside the mounted photos/ directory.
# Mounting photos/ (not photos/build/) means npm rebuilds—which delete and
# recreate build/—don't break the bind mount's inode binding.
RUN sed -i 's|DocumentRoot "/usr/local/apache2/htdocs"|DocumentRoot "/usr/local/apache2/htdocs/build"|g' /usr/local/apache2/conf/httpd.conf \
    && sed -i 's|<Directory "/usr/local/apache2/htdocs">|<Directory "/usr/local/apache2/htdocs/build">|g' /usr/local/apache2/conf/httpd.conf \
    && sed -i 's/#LoadModule rewrite_module/LoadModule rewrite_module/' /usr/local/apache2/conf/httpd.conf \
    && sed -i 's/AllowOverride None/AllowOverride All/g' /usr/local/apache2/conf/httpd.conf \
    && echo "ServerName localhost" >> /usr/local/apache2/conf/httpd.conf

# photos/ dir is mounted at runtime via -v; nothing to COPY here.
# docker run --rm -p 8080:80 -v $(PWD)/photos:/usr/local/apache2/htdocs:ro photos-apache
