---@author vhyrro
---@license GPLv3

local io = {}

io.write_to_wiki = function(filename, content)
    -- Ensure wiki directory exists
    local wiki_dir = "../wiki"
    vim.fn.mkdir(wiki_dir, "p")
    
    -- Debug output
    print("Writing to wiki: " .. filename .. ".md")
    print("Wiki directory: " .. wiki_dir)
    print("Content lines: " .. #content)
    
    vim.fn.writefile(content, wiki_dir .. "/" .. filename .. ".md")
end

return io
