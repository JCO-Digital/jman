import js from "@eslint/js";
import tseslint from "typescript-eslint";
import pluginVue from "eslint-plugin-vue";
import vueParser from "vue-eslint-parser";
import prettier from "eslint-config-prettier";
import prettierPlugin from "eslint-plugin-prettier";
import globals from "globals";

export default [
	// Global ignores
	{
		ignores: ["dist/**", "node_modules/**"],
	},

	// Base JS rules
	js.configs.recommended,

	// TypeScript rules
	...tseslint.configs.recommended,

	// Vue rules
	...pluginVue.configs["flat/recommended"],

	// Disable Vue formatting rules that conflict with Prettier
	prettier,

	// Vue + TypeScript parser config
	{
		files: ["**/*.vue"],
		languageOptions: {
			parser: vueParser,
			parserOptions: {
				parser: tseslint.parser,
				sourceType: "module",
			},
		},
	},

	// Project-specific rules
	{
		files: ["**/*.{ts,tsx,vue}"],
		languageOptions: {
			globals: {
				...globals.browser,
			},
		},
		plugins: {
			prettier: prettierPlugin,
		},
		rules: {
			// Prettier integration
			"prettier/prettier": "warn",

			// TypeScript
			"@typescript-eslint/no-explicit-any": "off",
			"@typescript-eslint/no-empty-object-type": "off",
			"@typescript-eslint/no-unused-vars": [
				"warn",
				{ argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
			],

			// Vue - disable formatting rules handled by Prettier
			"vue/multi-word-component-names": "off",
			"vue/no-v-html": "off",
			"vue/require-default-prop": "off",
			"vue/html-indent": "off",
			"vue/html-self-closing": "off",
			"vue/max-attributes-per-line": "off",
			"vue/singleline-html-element-content-newline": "off",
			"vue/multiline-html-element-content-newline": "off",
			"vue/html-closing-bracket-newline": "off",
			"vue/first-attribute-linebreak": "off",
		},
	},
];
