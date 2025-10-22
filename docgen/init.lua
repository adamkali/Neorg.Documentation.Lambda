print("=== DOCGEN INIT: Starting documentation generation ===")

local docgen = require("docgen")
local fileio = require("fileio")

print("DEBUG: Current working directory: " .. vim.fn.getcwd())
print("DEBUG: Files in current directory:")
for _, file in ipairs(vim.fn.glob("*", false, true)) do
    print("  " .. file)
end

local neorg = require("neorg.core")
local lib, modules = neorg.lib, neorg.modules

print("DEBUG: Neorg core loaded successfully")

--- CONFIGURABLE DOCGEN BEHAVIOUR
--- Tweak as you see fit.
local config = {
    --- When true, will auto-unfold the top-level <details>
    --- tags generated when rendering module configuration options.
    auto_open_first_level_tags = true,
}

---@type Modules
local doc_modules = {
    --[[
    [name] = {
        top_comment_data...
        buffer = id,
        parsed = `ret value from sourcing the file`,
    }
    --]]
}

--- Fully renders a large set of configuration options
---@param configuration_options ConfigOptionArray[] An array of ConfigOptionArrays
---@return string[] #An array of markdown strings corresponding to all of the rendered configuration options
local function concat_configuration_options(configuration_options)
    local result = {}

    local unrolled = lib.unroll(configuration_options)

    table.sort(unrolled, function(x, y)
        return x[1] < y[1]
    end)

    for _, values in pairs(unrolled) do
        vim.list_extend(result, docgen.render(values[2], config.auto_open_first_level_tags))
        table.insert(result, "")
    end

    return result
end

local module_files = docgen.aggregate_module_files()
print("DEBUG: Found " .. #module_files .. " module files:")
for i, file in ipairs(module_files) do
    print("  " .. i .. ": " .. file)
end

for _, file in ipairs(module_files) do
    local fullpath = vim.fn.fnamemodify(file, ":p")

    local buffer = docgen.open_file(fullpath)

    local top_comment = docgen.get_module_top_comment(buffer)

    if not top_comment then
        vim.notify("no top comment found for module " .. file)
        goto continue
    end

    local top_comment_data = docgen.check_top_comment_integrity(docgen.parse_top_comment(top_comment))

    if type(top_comment_data) == "string" then
        vim.notify("Error when parsing module '" .. file .. "': " .. top_comment_data)
        goto continue
    end

    -- Source the module file to retrieve some basic information like its name
    local ok, parsed_module = pcall(dofile, fullpath)

    if not ok then
        vim.notify("Error when sourcing module '" .. file .. "': " .. parsed_module)
        goto continue
    end

    -- Make Neorg load the module, which also evaluates dependencies
    local _ok, err = pcall(modules.load_module, parsed_module.name)

    if not _ok then
        vim.notify("Error when loading module '" .. file .. "': " .. err)
        goto continue
    end

    -- Retrieve the module from the `loaded_modules` table.
    parsed_module = modules.loaded_modules[parsed_module.name]

    if parsed_module then
        doc_modules[parsed_module.name] = {
            top_comment_data = top_comment_data,
            buffer = buffer,
            parsed = parsed_module,
        }
    end

    ::continue::
end

print("DEBUG: Processing " .. vim.tbl_count(doc_modules) .. " modules for documentation")
for name, _ in pairs(doc_modules) do
    print("  Module: " .. name)
end

-- Non-module pages have their own dedicated generators
print("DEBUG: Writing homepage")
fileio.write_to_wiki("Home", docgen.generators.homepage(doc_modules))

print("DEBUG: Writing keybinds")
fileio.write_to_wiki(
    "Default-Keybinds",
    docgen.generators.keybinds(
        doc_modules,
        docgen.open_file(vim.fn.fnamemodify("../lua/neorg/modules/core/keybinds/module.lua", ":p"))
    )
)

print("DEBUG: Writing sidebar")
fileio.write_to_wiki("_Sidebar", docgen.generators.sidebar(doc_modules))

-- Loop through all modules and generate their respective wiki files
print("DEBUG: Generating wiki files for individual modules")
for module_name, module in pairs(doc_modules) do
    print("DEBUG: Processing module: " .. module_name)
    local buffer = module.buffer

    -- Query the root node and try to find a `module.config.public` table
    local root = vim.treesitter.get_parser(buffer, "lua"):parse()[1]:root()
    local config_node = docgen.get_module_config_node(buffer, root)

    -- A collection of data about all the configuration options for the current module
    ---@type ConfigOptionArray[]
    local configuration_options = {}

    if config_node then
        docgen.map_config(buffer, config_node, function(data, comments)
            for i, comment in ipairs(comments) do
                comments[i] = docgen.lookup_modules(doc_modules, comment:gsub("^%s*%-%-+%s*", ""))
            end

            do
                local error = docgen.check_comment_integrity(table.concat(comments, "\n"))

                if type(error) == "string" then
                    -- Get the exact location of the error with data.node and the file it was contained in
                    local start_row, start_col = data.node:start()

                    vim.notify(
                        ("Error when parsing annotation in module '%s' on line (%d, %d): %s"):format(
                            module_name,
                            start_row,
                            start_col,
                            error
                        )
                    )
                    return
                end
            end

            if not data.value then
                return
            end

            local object = docgen.to_lua_object(module.parsed, buffer, data.value, module_name)

            do
                lib.ensure_nested(configuration_options, unpack(data.parents))
                local ref = vim.tbl_get(configuration_options, unpack(data.parents)) or configuration_options
                if data.name then
                    ref[data.name] = {
                        self = {
                            buffer = buffer,
                            data = data,
                            comments = comments,
                            object = object,
                        },
                    }
                else
                    table.insert(ref, {
                        self = {
                            buffer = buffer,
                            data = data,
                            comments = comments,
                            object = object,
                        },
                    })
                end
            end
        end)
    end

    -- Perform module lookups in the module's top comment markdown data.
    -- This cannot be done earlier because then there would be no guarantee
    -- that all the modules have been properly indexed and parsed.
    for i, line in ipairs(module.top_comment_data.markdown) do
        module.top_comment_data.markdown[i] = docgen.lookup_modules(doc_modules, line)
    end

    print("DEBUG: Writing wiki file for module: " .. module_name .. " -> " .. module.top_comment_data.file)
    fileio.write_to_wiki(
        module.top_comment_data.file,
        docgen.generators.module(doc_modules, module, concat_configuration_options(configuration_options))
    )
end

print("=== DOCGEN INIT: Documentation generation completed ===")
