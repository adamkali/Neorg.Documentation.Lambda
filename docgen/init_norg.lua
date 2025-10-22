print("=== NEORG DOCUMENT CONVERTER: Starting ===")

local fileio = require("fileio")
local converter = require("norg_converter")

print("DEBUG: Current working directory: " .. vim.fn.getcwd())
print("DEBUG: Files in current directory:")
for _, file in ipairs(vim.fn.glob("*", false, true)) do
    print("  " .. file)
end

-- Convert all .norg files to markdown
converter.convert_all()

print("=== NEORG DOCUMENT CONVERTER: Completed ===")