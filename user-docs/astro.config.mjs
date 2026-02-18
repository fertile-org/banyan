import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

export default defineConfig({
  integrations: [
    starlight({
      title: "Banyan",
      description: "Docker Compose that scales.",
      logo: {
        src: "./src/assets/logo.png",
        alt: "Banyan",
      },
      social: {
        github: "https://github.com/fertile-org/banyan",
      },
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
      ],
    }),
  ],
});
