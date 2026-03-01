import { defineConfig } from "astro/config";
import mermaid from "astro-mermaid";
import starlight from "@astrojs/starlight";

export default defineConfig({
  site: "https://getbanyan.dev",
  integrations: [
    mermaid({
      theme: "forest",
    }),
    starlight({
      title: "Banyan",
      description: "Container orchestration you already know.",
      favicon: "/og-image.png",
      logo: {
        src: "./src/assets/logo.png",
        alt: "Banyan",
      },
      social: {
        github: "https://github.com/fertile-org/banyan",
      },
      head: [
        {
          tag: "meta",
          attrs: { name: "theme-color", content: "#5b8c2a" },
        },
        {
          tag: "meta",
          attrs: { property: "og:type", content: "website" },
        },
        {
          tag: "meta",
          attrs: { property: "og:site_name", content: "Banyan" },
        },
        {
          tag: "meta",
          attrs: {
            property: "og:image",
            content: "https://getbanyan.dev/og-image.png",
          },
        },
        {
          tag: "meta",
          attrs: { name: "twitter:card", content: "summary" },
        },
        {
          tag: "meta",
          attrs: {
            name: "keywords",
            content:
              "banyan, container orchestration, docker compose, deployment, self-hosted, distributed systems",
          },
        },
      ],
      customCss: ["./src/styles/custom.css"],
      sidebar: [
        {
          label: "Getting Started",
          autogenerate: { directory: "getting-started" },
        },
        {
          label: "Guides",
          autogenerate: { directory: "guides" },
        },
        {
          label: "Reference",
          autogenerate: { directory: "reference" },
        },
        {
          label: "Roadmap",
          slug: "roadmap",
        },
        {
          label: "White Paper",
          slug: "whitepaper",
        },
      ],
    }),
  ],
});
