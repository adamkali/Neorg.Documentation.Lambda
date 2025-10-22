" Copied from: https://github.com/ThePrimeagen/refactoring.nvim/blob/master/scripts/minimal.vim

" Current neorg code
set rtp+=.

" For test suites - use installed plugins from Lazy
set rtp+=/app/data/nvim/lazy/nvim-treesitter
set rtp+=/app/data/nvim/lazy/neorg

set noswapfile

runtime! plugin/nvim-treesitter.vim

lua << EOF
-- Add the lazy plugin paths to package.path
local lazy_path = "/app/data/nvim/lazy"
package.path = lazy_path .. "/nvim-treesitter/lua/?.lua;" .. package.path
package.path = lazy_path .. "/nvim-treesitter/lua/?/init.lua;" .. package.path
package.path = lazy_path .. "/neorg/lua/?.lua;" .. package.path
package.path = lazy_path .. "/neorg/lua/?/init.lua;" .. package.path
package.path = "../lua/?.lua;" .. "../lua/?/init.lua;" .. package.path
package.path = "/usr/share/lua/5.1/?.lua;/usr/share/lua/5.1/?/init.lua;" .. package.path
package.path = "/usr/share/lua/5.1/?.so;" .. package.path

require("nvim-treesitter").setup({})

local ok, module = pcall(require,'nvim-treesitter.configs')
if ok then
    module.setup({})
end

vim.cmd.TSInstallSync({
    bang = true,
    args = { "lua", "norg" },
})
EOF
