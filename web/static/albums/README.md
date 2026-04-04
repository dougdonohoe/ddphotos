# albums placeholder

This directory exists so that `adapter-static` copies it to `web/build/albums/`,
giving Docker a mountpoint for the second `-v` bind mount at runtime.

The actual album data (JSON + images) lives outside the build, in the directory
pointed to by `DDPHOTOS_ALBUMS_DIR/DDPHOTOS_SITE_ID` (see `config/defaults.env`).
