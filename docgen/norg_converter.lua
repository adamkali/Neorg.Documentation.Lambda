-- Neorg to Markdown converter
local fileio = require("fileio")

local converter = {}

-- Initialize Neorg for document parsing
require("neorg").setup({
    load = {
        ["core.defaults"] = {},
        ["core.export"] = {},
        ["core.export.markdown"] = {},
        ["core.dirman"] = {
            config = {
                workspaces = {
                    notes = ".",
                },
                default_workspace = "notes",
            },
        },
    },
})

-- Function to find all .norg files
function converter.find_norg_files()
    local files = vim.fs.find("**/*.norg", {
        path = "..",
        type = "file",
        limit = math.huge,
    })
    
    print("DEBUG: Found " .. #files .. " .norg files:")
    for i, file in ipairs(files) do
        print("  " .. i .. ": " .. file)
    end
    
    return files
end

-- Function to convert a single .norg file to markdown
function converter.convert_norg_to_markdown(norg_file)
    print("DEBUG: Converting " .. norg_file .. " to markdown")
    
    -- Read the .norg file
    local content = vim.fn.readfile(norg_file)
    if not content then
        print("ERROR: Could not read file " .. norg_file)
        return nil
    end
    
    -- Basic Neorg to Markdown conversion
    local markdown_lines = {}
    local in_code_block = false
    local code_lang = ""
    
    for _, line in ipairs(content) do
        -- Skip document.meta blocks
        if line:match("^@document%.meta") then
            goto continue
        elseif line:match("^@end") then
            goto continue
        end
        
        -- Handle code blocks
        if line:match("^%s*@code") then
            code_lang = line:match("@code%s*(%w*)")
            table.insert(markdown_lines, "```" .. (code_lang or ""))
            in_code_block = true
            goto continue
        elseif line:match("^%s*@end") and in_code_block then
            table.insert(markdown_lines, "```")
            in_code_block = false
            goto continue
        end
        
        if in_code_block then
            table.insert(markdown_lines, line)
            goto continue
        end
        
        -- Convert headers (* -> #, ** -> ##, etc.)
        local header_level = line:match("^(%*+)")
        if header_level then
            local header_text = line:match("^%*+%s*(.*)")
            local md_header = string.rep("#", #header_level) .. " " .. header_text
            table.insert(markdown_lines, md_header)
            goto continue
        end
        
        -- Convert list items (- -> -, ~ -> 1.)
        local list_indent = line:match("^(%s*)")
        local list_marker = line:match("^%s*([%-~]+)")
        if list_marker then
            local list_text = line:match("^%s*[%-~]+%s*(.*)")
            if list_marker:match("^%-+$") then
                -- Unordered list
                local indent_level = (#list_marker - 1) * 2
                local md_line = string.rep(" ", indent_level) .. "- " .. list_text
                table.insert(markdown_lines, md_line)
            else
                -- Ordered list
                local indent_level = (#list_marker - 1) * 2
                local md_line = string.rep(" ", indent_level) .. "1. " .. list_text
                table.insert(markdown_lines, md_line)
            end
            goto continue
        end
        
        -- Convert inline formatting
        local converted_line = line
        -- Bold: {* text *} -> **text**
        converted_line = converted_line:gsub("{%*%s*(.-)%s*%*}", "**%1**")
        -- Italic: {/ text /} -> *text*
        converted_line = converted_line:gsub("{/%s*(.-)%s*/}", "*%1*")
        -- Code: {` text `} -> `text`
        converted_line = converted_line:gsub("{`%s*(.-)%s*`}", "`%1`")
        -- Strikethrough: {- text -} -> ~~text~~
        converted_line = converted_line:gsub("{%-%s*(.-)%s*%-}", "~~%1~~")
        -- Links: {url}[text] -> [text](url)
        converted_line = converted_line:gsub("{([^}]+)}%[([^%]]+)%]", "[%2](%1)")
        
        table.insert(markdown_lines, converted_line)
        
        ::continue::
    end
    
    return markdown_lines
end

-- Function to convert all .norg files to markdown
function converter.convert_all()
    print("=== NEORG TO MARKDOWN CONVERTER: Starting conversion ===")
    
    local norg_files = converter.find_norg_files()
    
    if #norg_files == 0 then
        print("No .norg files found")
        return
    end
    
    for _, norg_file in ipairs(norg_files) do
        local markdown_content = converter.convert_norg_to_markdown(norg_file)
        
        if markdown_content then
            -- Generate output filename
            local base_name = norg_file:match("([^/]+)%.norg$") or "unknown"
            local output_file = base_name
            
            print("DEBUG: Writing markdown to " .. output_file .. ".md")
            fileio.write_to_wiki(output_file, markdown_content)
        end
    end
    
    print("=== NEORG TO MARKDOWN CONVERTER: Conversion completed ===")
end

return converter