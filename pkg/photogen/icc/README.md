# Bundled ICC profiles

`DisplayP3-v4.icc` and `Rec2020-v4.icc` are from
[saucecontrol/Compact-ICC-Profiles](https://github.com/saucecontrol/Compact-ICC-Profiles),
released under CC0 1.0. They are embedded in photogen (`color.go`) as the input profile for
AVIF/HEIF files that describe their color with an nclx tag, which libvips ignores.
