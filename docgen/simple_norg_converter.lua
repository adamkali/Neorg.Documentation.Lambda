-- Simple Neorg to Markdown converter without full Neorg setup
local fileio = require("fileio")

print("=== SIMPLE NEORG TO MARKDOWN CONVERTER: Starting ===")

print("DEBUG: Current working directory: " .. vim.fn.getcwd())
print("DEBUG: Files in current directory:")
for _, file in ipairs(vim.fn.glob("*", false, true)) do
    print("  " .. file)
end

-- Function to find all .norg files recursively
local function find_norg_files(dir)
    dir = dir or ".."
    local files = {}
    
    local function scan_dir(path)
        local items = vim.fn.glob(path .. "/*", false, true)
        for _, item in ipairs(items) do
            if vim.fn.isdirectory(item) == 1 then
                scan_dir(item)
            elseif item:match("%.norg$") then
                table.insert(files, item)
            end
        end
    end
    
    scan_dir(dir)
    return files
end

-- Function to convert a single .norg file to markdown
local function convert_norg_to_markdown(norg_file)
    print("DEBUG: Converting " .. norg_file .. " to markdown")
    
    -- Read the .norg file
    local content = vim.fn.readfile(norg_file)
    if not content then
        print("ERROR: Could not read file " .. norg_file)
        return nil
    end
    
    -- Check if file has a top-level header (starts with single *)
    local has_top_level_header = false
    for _, line in ipairs(content) do
        if line:match("^%*[^%*]") then -- Single * NOT followed by another *
            has_top_level_header = true
            break
        end
    end
    
    -- Basic Neorg to Markdown conversion
    local markdown_lines = {}
    local in_code_block = false
    local in_meta_block = false
    local code_lang = ""
    
    for _, line in ipairs(content) do
        -- Handle document.meta blocks
        if line:match("^@document%.meta") then
            in_meta_block = true
            goto continue
        elseif line:match("^@end") and in_meta_block then
            in_meta_block = false
            goto continue
        elseif in_meta_block then
            -- Extract title from meta if available, but only if no top-level header exists
            local title = line:match("^title:%s*(.+)")
            if title and not has_top_level_header then
                table.insert(markdown_lines, "# " .. title)
                table.insert(markdown_lines, "")
            end
            goto continue
        end
        
        -- Handle code blocks
        if line:match("^%s*@code") then
            code_lang = line:match("@code%s*(%w*)") or ""
            table.insert(markdown_lines, "```" .. code_lang)
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
        local header_match = line:match("^(%*+)%s*(.*)")
        if header_match then
            local header_level, header_text = line:match("^(%*+)%s*(.*)")
            if header_level and header_text then
                local md_header = string.rep("#", #header_level) .. " " .. header_text
                table.insert(markdown_lines, md_header)
                table.insert(markdown_lines, "")
                goto continue
            end
        end
        
        -- Convert TODO items with task status
        local todo_match = line:match("^(%s*)([%-~%*]+)%s*%((.-)%)%s*(.*)")
        if todo_match then
            local indent, marker, status, text = line:match("^(%s*)([%-~%*]+)%s*%((.-)%)%s*(.*)")
            if marker and status and text then
                local indent_level = math.max(0, (#marker - 1) * 2)
                -- Check if task is completed (x) or unchecked (empty or just spaces)
                local checkbox = (status == "" or status:match("^%s*$")) and "[ ]" or "[x]"
                local md_line = string.rep(" ", indent_level) .. "- " .. checkbox .. " " .. text
                table.insert(markdown_lines, md_line)
                goto continue
            end
        end
        
        -- Convert regular list items
        local list_match = line:match("^(%s*)([%-~]+)%s*(.*)")
        if list_match then
            local indent, marker, text = line:match("^(%s*)([%-~]+)%s*(.*)")
            if marker and text then
                local indent_level = math.max(0, (#marker - 1) * 2)
                local list_char = marker:match("^%-") and "-" or "1."
                local md_line = string.rep(" ", indent_level) .. list_char .. " " .. text
                table.insert(markdown_lines, md_line)
                goto continue
            end
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

-- Find and convert all .norg files
local norg_files = find_norg_files()

print("DEBUG: Found " .. #norg_files .. " .norg files:")
for i, file in ipairs(norg_files) do
    print("  " .. i .. ": " .. file)
end

if #norg_files == 0 then
    print("No .norg files found - creating a sample markdown file")
    fileio.write_to_wiki("README", {
        "# No Neorg Files Found",
        "",
        "This directory did not contain any `.norg` files to convert.",
        "",
        "To use this converter, include `.norg` files in your project archive.",
    })
else
    for _, norg_file in ipairs(norg_files) do
        local markdown_content = convert_norg_to_markdown(norg_file)
        
        if markdown_content then
            -- Generate output filename
            local base_name = norg_file:match("([^/]+)%.norg$") or "unknown"
            
            print("DEBUG: Writing markdown to " .. base_name .. ".md")
            fileio.write_to_wiki(base_name, markdown_content)
        end
    end
end

print("=== SIMPLE NEORG TO MARKDOWN CONVERTER: Completed ===")