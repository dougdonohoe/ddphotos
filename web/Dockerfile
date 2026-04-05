FROM httpd:2.4

# Enable mod_rewrite, AllowOverride for .htaccess, FollowSymLinks for entrypoint symlinks.
RUN sed -i 's/#LoadModule rewrite_module/LoadModule rewrite_module/' /usr/local/apache2/conf/httpd.conf \
    && sed -i 's/AllowOverride None/AllowOverride All/g' /usr/local/apache2/conf/httpd.conf \
    && echo "ServerName localhost" >> /usr/local/apache2/conf/httpd.conf \
    && printf '\n<Directory "/usr/local/apache2/htdocs">\n    Options +FollowSymLinks\n</Directory>\n' >> /usr/local/apache2/conf/httpd.conf

# entrypoint.sh populates /usr/local/apache2/htdocs with symlinks into /build/<site-id>
# and /albums at container startup, then hands off to httpd-foreground.
COPY entrypoint.sh /entrypoint.sh
CMD ["/entrypoint.sh"]

# Mounts at runtime:
#   -v <root>/build:/build:ro
#   -v <root>/albums/<site-id>:/albums:ro
#   -e DDPHOTOS_SITE_ID=<site-id>
