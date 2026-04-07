FROM nginx:alpine

# Replace the default config with our routing rules.
COPY nginx.conf /etc/nginx/conf.d/default.conf

# nginx-entrypoint.sh calls setup-htdocs.sh to populate the document root with symlinks,
# then hands off to nginx.
COPY setup-htdocs.sh /setup-htdocs.sh
COPY nginx-entrypoint.sh /entrypoint.sh
RUN chmod +x /setup-htdocs.sh /entrypoint.sh
CMD ["/entrypoint.sh"]

# Mounts at runtime:
#   -v <root>/build:/build:ro
#   -v <root>/albums/<site-id>:/albums:ro
#   -e DDPHOTOS_SITE_ID=<site-id>
