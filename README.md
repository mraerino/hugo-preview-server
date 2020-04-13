# Hugo Preview Server

**This is in an early stage:** It has only been tested with a single demo site and misses some features.

## Summary

A piece of glue code that runs Hugo for serving previews for CMS systems.

It uses a git client to get access to assets/layouts of the site and compiles pages based on content passed in via HTTP.

## Features

- Uses GitHub as a virtual filesystem to get layouts
- Renders pages on-demand, taking `path` and `data` via `POST`
- Supports partial re-renders (just like `hugo server`)
- Supports extended mode

## Roadmap

- Build & package a solid integration for [NetlifyCMS](https://www.netlifycms.org/)
- Support custom source branches (only default right now)
- Provide tooling for easy usage (e.g. `npx hugo-preview-server setup:function ./functions`)
- Run as a standalone http server process (for use in docker)

## Usage

There is a binary available that can be deployed to AWS Lambda or any compatible platforms like [Netlify Functions](https://www.netlify.com/products/functions/).

Use this command in your build to get the latest release:

```
curl -L https://github.com/mraerino/hugo-preview-server/releases/latest/download/preview-lambda -o <destination>

# Example for Netlify Functions
mkdir -p functions
curl -L https://github.com/mraerino/hugo-preview-server/releases/latest/download/preview-lambda -o functions/preview
```

See [this `Netlify.toml`](demo/netlify.toml) for an example

In the future there will be tooling that will make this even easier.

## License

See [LICENSE file](LICENSE.md)
