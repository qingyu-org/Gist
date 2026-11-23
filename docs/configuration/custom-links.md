# Custom links

The bottom of the sidebar can hold links to your own pages or to other
websites — useful for legal information, a privacy policy, a personal site or a
blog.

## Admin panel (recommended)

Open **Admin → Configuration → Site → Sidebar links** and add a row for each
link. Every row has an icon, a label and a URL. Changes are stored in the
database and apply immediately.

The icon list includes a generic link, home, website, GitHub, GitLab, Gitea,
email, RSS, code, blog, profile, X, Mastodon and Telegram. Relative paths such
as `/privacy` and `mailto:` links are accepted as well as `http(s)` URLs.

The source-code icon at the very bottom of the sidebar is controlled separately
by **Source repository**. Leave it empty to hide the icon.

## Configuration file (legacy)

The `custom.static-links` configuration is still supported for existing
instances and for links that point at custom HTML files. These links use a
generic link icon.

If you have a legal information file `legal.html` in the `$qingyugist-home/custom`
directory, and also wish to add a link to a Gitea instance, set the links as
follows:

#### YAML
```yaml
custom.static-links:
  - name: Legal notices
    path: legal.html
  - name: Gitea
    path: https://gitea.com
```

#### Environment variable
```sh
QYG_CUSTOM_STATIC_LINK_0_NAME="Legal Notices" \
QYG_CUSTOM_STATIC_LINK_0_PATH=legal.html \
QYG_CUSTOM_STATIC_LINK_1_NAME=Gitea \
QYG_CUSTOM_STATIC_LINK_1_PATH=https://gitea.com \
./qingyugist
```

> [!Note]
> Like the other files in the `$opengist-home/custom` directory, custom pages can be viewed without logging in, even when the "Require login" setting is enabled.

## Templating custom HTML pages

In the start and end of the custom HTML files, you can use the syntax to include the header and footer of the QingyuGist instance:

```html
{{ template "header" . }}

<!-- my content -->

{{ template "footer" . }}
```

If you want your custom page to integrate well into the existing theme, you can use the following:

```html
{{ template "header" . }}

<div class="py-10">
   <header class="pb-4 ">
      <div class="flex">
         <div class="flex-auto">
            <h2 class="text-2xl font-bold leading-tight">Heading</h2>
         </div>
      </div>
   </header>
   <main>
     <h3 class="text-xl font-bold leading-tight mt-4">Sub-Heading</h3>
     <p class="mt-4 ml-1"><!-- my content --></p>
   </main>
</div>

{{ template "footer" . }}
```

You can adjust above as needed. QingyuGist uses TailwindCSS classes.
