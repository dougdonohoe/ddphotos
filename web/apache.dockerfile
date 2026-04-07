FROM httpd:2.4

# Enable mod_rewrite, AllowOverride for .htaccess, FollowSymLinks for entrypoint symlinks.
RUN sed -i 's/#LoadModule rewrite_module/LoadModule rewrite_module/' /usr/local/apache2/conf/httpd.conf \
    && sed -i 's/AllowOverride None/AllowOverride All/g' /usr/local/apache2/conf/httpd.conf \
    && echo "ServerName localhost" >> /usr/local/apache2/conf/httpd.conf \
    && printf '\n<Directory "/usr/local/apache2/htdocs">\n    Options +FollowSymLinks\n</Directory>\n' >> /usr/local/apache2/conf/httpd.conf

# apache-entrypoint.sh calls setup-htdocs.sh to populate the document root with symlinks,
# then hands off to httpd-foreground.
COPY setup-htdocs.sh /setup-htdocs.sh
COPY apache-entrypoint.sh /entrypoint.sh
RUN chmod +x /setup-htdocs.sh /entrypoint.sh
CMD ["/entrypoint.sh"]

# Mounts at runtime:
#   -v <root>/build:/build:ro
#   -v <root>/albums/<site-id>:/albums:ro
#   -e DDPHOTOS_SITE_ID=<site-id>
